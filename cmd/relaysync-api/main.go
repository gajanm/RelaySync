package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"relaysync/internal/api"
	"relaysync/internal/cache"
	"relaysync/internal/config"
	"relaysync/internal/logging"
	"relaysync/internal/metrics"
	"relaysync/internal/notifier"
	"relaysync/internal/pipeline"
	"relaysync/internal/store/postgres"
)

func main() {
	logger := logging.NewLogger(getEnv("APP_ENV", "dev"))

	if err := run(logger); err != nil {
		logger.Error("application failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	// ---------------------------------------------------------
	// Configuration
	// ---------------------------------------------------------

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Log only safe, useful config values.
	// Avoid logging credentials / connection URLs.
	logger.Info(
		"config loaded",
		"http_addr", cfg.HTTPAddr,
		"redis_stream", cfg.RedisStream,
		"redis_group", cfg.RedisGroup,
		"redis_consumer", cfg.RedisConsumer,
		"fcm_enabled", cfg.FCMEnabled,
	)

	// ---------------------------------------------------------
	// Application lifecycle
	// ---------------------------------------------------------

	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	// ---------------------------------------------------------
	// PostgreSQL
	// ---------------------------------------------------------

	store, err := postgres.New(ctx, cfg.PostgresURL)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer store.Close()

	logger.Info("postgres connected")

	// ---------------------------------------------------------
	// Redis
	// ---------------------------------------------------------

	cacheClient, err := cache.NewClient(
		cfg.RedisURL,
		cfg.RedisStream,
	)
	if err != nil {
		return fmt.Errorf("connect to redis: %w", err)
	}

	logger.Info("redis connected")

	// ---------------------------------------------------------
	// Notification service
	// ---------------------------------------------------------

	notifierClient, err := buildNotifier(ctx, logger, cfg)
	if err != nil {
		return fmt.Errorf("build notifier: %w", err)
	}

	// ---------------------------------------------------------
	// Prometheus metrics
	// ---------------------------------------------------------

	registry := prometheus.NewRegistry()
	metrics.Register(registry)

	// ---------------------------------------------------------
	// Location processing pipeline
	// ---------------------------------------------------------

	consumer := pipeline.NewConsumer(
		logger,
		cacheClient.Client(),
		store,
		notifierClient,
		cfg.RedisStream,
		cfg.RedisGroup,
		cfg.RedisConsumer,
		cfg.StreamBatchSize,
		cfg.StreamBatchWindow,
		cfg.StreamBlock,
		cfg.NotificationQueue,
		cfg.NotifierWorkers,
		cfg.FCMTopicPrefix,
	)

	if err := consumer.Start(ctx); err != nil {
		return fmt.Errorf("start pipeline consumer: %w", err)
	}

	logger.Info(
		"pipeline consumer started",
		"stream", cfg.RedisStream,
		"group", cfg.RedisGroup,
		"consumer", cfg.RedisConsumer,
	)

	// ---------------------------------------------------------
	// API
	// ---------------------------------------------------------

	server := api.NewServer(
		logger,
		cfg,
		store,
		cacheClient,
	)

	handler := promhttpHandler(
		cfg.MetricsPath,
		registry,
		server.Handler(),
	)

	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       cfg.APITimeout,
		WriteTimeout:      cfg.APITimeout,
		IdleTimeout:       30 * time.Second,
	}

	// Buffered so the HTTP goroutine can report an error
	// without blocking.
	serverErr := make(chan error, 1)

	go func() {
		logger.Info(
			"http server started",
			"addr", cfg.HTTPAddr,
		)

		err := httpServer.ListenAndServe()

		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- fmt.Errorf("listen and serve: %w", err)
		}
	}()

	// ---------------------------------------------------------
	// Wait for termination
	// ---------------------------------------------------------

	var runErr error

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")

	case err := <-serverErr:
		runErr = err

		logger.Error(
			"http server failed",
			"error", err,
		)

		// Cancel application context so background workers
		// also receive the shutdown signal.
		stop()
	}

	// ---------------------------------------------------------
	// Graceful shutdown
	// ---------------------------------------------------------

	logger.Info("shutdown initiated")

	shutdownCtx, cancelShutdown := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancelShutdown()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}

	logger.Info("http server stopped")
	logger.Info("shutdown complete")

	// If the HTTP server crashed unexpectedly, propagate that
	// failure so main exits with a non-zero exit code.
	if runErr != nil {
		return runErr
	}

	return nil
}

func buildNotifier(
	ctx context.Context,
	logger *slog.Logger,
	cfg config.Config,
) (notifier.Notifier, error) {
	if !cfg.FCMEnabled {
		logger.Info("using mock notifier")
		return notifier.NewMock(logger), nil
	}

	credentials := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")

	fcmClient, err := notifier.NewFCM(
		ctx,
		cfg.FCMProjectID,
		credentials,
	)
	if err != nil {
		return nil, fmt.Errorf("initialize FCM notifier: %w", err)
	}

	logger.Info(
		"using fcm notifier",
		"project_id", cfg.FCMProjectID,
	)

	return fcmClient, nil
}

func promhttpHandler(
	metricsPath string,
	registry *prometheus.Registry,
	next http.Handler,
) http.Handler {
	// Build this once instead of creating a new Prometheus
	// handler for every incoming metrics request.
	metricsHandler := promhttp.HandlerFor(
		registry,
		promhttp.HandlerOpts{},
	)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == metricsPath {
			metricsHandler.ServeHTTP(w, r)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return defaultValue
}