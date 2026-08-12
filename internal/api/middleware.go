package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type key int

const requestIDKey key = iota

func withRequestID(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-ID")
			if id == "" {
				id = time.Now().UTC().Format("20060102T150405.000000")
			}
			ctx := context.WithValue(r.Context(), requestIDKey, id)
			w.Header().Set("X-Request-ID", id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			reqID := RequestID(r.Context())
			logger.Info("request", "method", r.Method, "path", r.URL.Path, "request_id", reqID)
			next.ServeHTTP(w, r)
			logger.Info("response", "method", r.Method, "path", r.URL.Path, "request_id", reqID, "duration_ms", time.Since(start).Milliseconds())
		})
	}
}

func RequestID(ctx context.Context) string {
	if val, ok := ctx.Value(requestIDKey).(string); ok {
		return val
	}
	return ""
}

type RateLimiter struct {
	limiters map[string]*rate.Limiter
	mu       sync.Mutex
	limit    rate.Limit
	burst    int
}

func NewRateLimiter(limit float64, burst int) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		limit:    rate.Limit(limit),
		burst:    burst,
	}
}

func (r *RateLimiter) getLimiter(key string) *rate.Limiter {
	r.mu.Lock()
	defer r.mu.Unlock()
	limiter, ok := r.limiters[key]
	if !ok {
		limiter = rate.NewLimiter(r.limit, r.burst)
		r.limiters[key] = limiter
	}
	return limiter
}

func (r *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		apiKey := req.Header.Get("X-API-Key")
		if apiKey == "" {
			apiKey = "anonymous"
		}
		courierID := strings.TrimSpace(req.URL.Query().Get("courier_id"))
		if courierID == "" && strings.HasPrefix(req.URL.Path, "/couriers/") {
			path := strings.TrimPrefix(req.URL.Path, "/couriers/")
			parts := strings.Split(path, "/")
			if len(parts) > 0 && parts[0] != "" && parts[0] != "nearby" {
				courierID = parts[0]
			}
		}
		if courierID == "" {
			courierID = "global"
		}
		key := apiKey + ":" + courierID
		if !r.getLimiter(key).Allow() {
			writeJSON(w, http.StatusTooManyRequests, errorResponse{"rate limit exceeded"})
			return
		}
		next.ServeHTTP(w, req)
	})
}

func apiKeyMiddleware(expected string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if expected == "" {
				next.ServeHTTP(w, r)
				return
			}
			if r.Header.Get("X-API-Key") != expected {
				writeJSON(w, http.StatusUnauthorized, errorResponse{"invalid api key"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
