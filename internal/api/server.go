package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"relaysync/internal/cache"
	"relaysync/internal/config"
	"relaysync/internal/ingest"
	"relaysync/internal/metrics"
	"relaysync/internal/store/postgres"
)

type Server struct {
	logger *slog.Logger
	cfg    config.Config
	store  *postgres.Store
	cache  *cache.Client
	router chi.Router
}

type errorResponse struct {
	Error string `json:"error"`
}

type courierRequest struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type locationRequest struct {
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
	AccuracyM float64 `json:"accuracy_m"`
	Timestamp string  `json:"timestamp"`
}

type locationResponse struct {
	CourierID  string    `json:"courier_id"`
	Lat        float64   `json:"lat"`
	Lng        float64   `json:"lng"`
	AccuracyM  float64   `json:"accuracy_m"`
	RecordedAt time.Time `json:"recorded_at"`
	ServerTime time.Time `json:"server_time"`
	EventID    string    `json:"event_id"`
}

type subscriptionRequest struct {
	CourierID string `json:"courier_id"`
	Topic     string `json:"fcm_topic"`
}

func NewServer(logger *slog.Logger, cfg config.Config, store *postgres.Store, cache *cache.Client) *Server {
	server := &Server{logger: logger, cfg: cfg, store: store, cache: cache}
	server.router = server.routes()
	return server
}

func (s *Server) routes() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(withRequestID(s.logger))
	r.Use(requestLogger(s.logger))
	r.Use(middleware.Timeout(s.cfg.APITimeout))
	limiter := NewRateLimiter(s.cfg.RateLimitPerSecond, s.cfg.RateLimitBurst)
	r.Use(limiter.Middleware)

	r.Get("/healthz", s.health)
	r.Get("/readyz", s.ready)
	r.Get("/docs", s.docs)
	r.Get("/openapi.yaml", s.openapi)

	r.Route("/couriers", func(r chi.Router) {
		r.Use(apiKeyMiddleware(s.cfg.APIKey))
		r.Post("/", s.createCourier)
		r.Get("/", s.listCouriers)
		r.Get("/{id}", s.getCourier)
		r.Post("/{id}/location", s.ingestLocation)
		r.Get("/{id}/location/latest", s.latestLocation)
		r.Get("/{id}/location/history", s.locationHistory)
	})

	r.With(apiKeyMiddleware(s.cfg.APIKey)).Get("/couriers/nearby", s.nearbyCouriers)

	r.Route("/subscriptions", func(r chi.Router) {
		r.Use(apiKeyMiddleware(s.cfg.APIKey))
		r.Post("/", s.createSubscription)
		r.Delete("/{id}", s.deleteSubscription)
		r.Get("/", s.listSubscriptions)
	})

	return r
}

func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{"postgres unavailable"})
		return
	}
	if err := s.cache.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{"redis unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) docs(w http.ResponseWriter, r *http.Request) {
	html := `<!doctype html><html><head><title>RelaySync API Docs</title><link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css"></head><body><div id="swagger-ui"></div><script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script><script>window.onload=function(){SwaggerUIBundle({url:'/openapi.yaml',dom_id:'#swagger-ui'});}</script></body></html>`
	w.Header().Set("Content-Type", "text/html")
	_, _ = w.Write([]byte(html))
}

func (s *Server) openapi(w http.ResponseWriter, r *http.Request) {
	data, err := loadOpenAPI()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{"failed to load openapi"})
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	_, _ = w.Write(data)
}

func (s *Server) createCourier(w http.ResponseWriter, r *http.Request) {
	var req courierRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{"invalid payload"})
		return
	}
	if req.ID == "" {
		req.ID = uuid.NewString()
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{"name required"})
		return
	}
	if req.Status == "" {
		req.Status = "active"
	}
	courier, err := s.store.CreateCourier(r.Context(), postgres.Courier{ID: req.ID, Name: req.Name, Status: req.Status})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{"failed to create courier"})
		return
	}
	writeJSON(w, http.StatusCreated, courier)
}

func (s *Server) getCourier(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	courier, err := s.store.GetCourier(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{"courier not found"})
		return
	}
	writeJSON(w, http.StatusOK, courier)
}

func (s *Server) listCouriers(w http.ResponseWriter, r *http.Request) {
	limit := parseIntDefault(r.URL.Query().Get("limit"), 20)
	offset := parseIntDefault(r.URL.Query().Get("offset"), 0)
	if limit > 100 {
		limit = 100
	}
	couriers, err := s.store.ListCouriers(r.Context(), limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{"failed to list couriers"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": couriers, "limit": limit, "offset": offset})
}

func (s *Server) ingestLocation(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	id := chi.URLParam(r, "id")
	var req locationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		metrics.IngestRequestsTotal.WithLabelValues("bad_request").Inc()
		writeJSON(w, http.StatusBadRequest, errorResponse{"invalid payload"})
		return
	}
	if err := ingest.ValidateCoordinates(req.Lat, req.Lng); err != nil {
		metrics.IngestRequestsTotal.WithLabelValues("bad_request").Inc()
		writeJSON(w, http.StatusBadRequest, errorResponse{err.Error()})
		return
	}
	recordedAt, err := ingest.ParseTimestamp(req.Timestamp)
	if err != nil {
		metrics.IngestRequestsTotal.WithLabelValues("bad_request").Inc()
		writeJSON(w, http.StatusBadRequest, errorResponse{err.Error()})
		return
	}
	location := cache.Location{
		CourierID:  id,
		Lat:        req.Lat,
		Lng:        req.Lng,
		AccuracyM:  req.AccuracyM,
		RecordedAt: recordedAt,
		Source:     "ingest",
		Metadata:   map[string]string{"request_id": RequestID(r.Context())},
		EventID:    uuid.NewString(),
	}

	redisStart := time.Now()
	redisCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.cache.WriteLatest(redisCtx, location, s.cfg.LocationTTL); err != nil {
		metrics.IngestRequestsTotal.WithLabelValues("redis_error").Inc()
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{"redis write failed"})
		return
	}
	metrics.RedisWriteLatency.Observe(time.Since(redisStart).Seconds())

	if _, err := s.cache.AddStreamEvent(redisCtx, location); err != nil {
		metrics.IngestRequestsTotal.WithLabelValues("stream_error").Inc()
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{"stream enqueue failed"})
		return
	}
	metrics.IngestRequestsTotal.WithLabelValues("accepted").Inc()
	metrics.IngestLatency.Observe(time.Since(start).Seconds())

	writeJSON(w, http.StatusAccepted, locationResponse{
		CourierID:  id,
		Lat:        location.Lat,
		Lng:        location.Lng,
		AccuracyM:  location.AccuracyM,
		RecordedAt: location.RecordedAt,
		ServerTime: time.Now().UTC(),
		EventID:    location.EventID,
	})
}

func (s *Server) latestLocation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	loc, err := s.cache.GetLatest(r.Context(), id)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			writeJSON(w, http.StatusNotFound, errorResponse{"no latest location"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{"failed to get latest"})
		return
	}
	writeJSON(w, http.StatusOK, loc)
}

func (s *Server) locationHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	from := parseTimeDefault(r.URL.Query().Get("from"), time.Now().Add(-1*time.Hour))
	to := parseTimeDefault(r.URL.Query().Get("to"), time.Now())
	limit := parseIntDefault(r.URL.Query().Get("limit"), 100)
	if limit > 500 {
		limit = 500
	}
	items, err := s.store.GetLocationHistory(r.Context(), id, from, to, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{"failed to get history"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

func (s *Server) nearbyCouriers(w http.ResponseWriter, r *http.Request) {
	lat, err := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	if err != nil || math.IsNaN(lat) {
		writeJSON(w, http.StatusBadRequest, errorResponse{"lat required"})
		return
	}
	lng, err := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	if err != nil || math.IsNaN(lng) {
		writeJSON(w, http.StatusBadRequest, errorResponse{"lng required"})
		return
	}
	radius, err := strconv.ParseFloat(r.URL.Query().Get("radius_m"), 64)
	if err != nil || radius <= 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{"radius_m required"})
		return
	}
	limit := int64(parseIntDefault(r.URL.Query().Get("limit"), 50))
	if limit > 200 {
		limit = 200
	}
	items, err := s.cache.Nearby(r.Context(), lat, lng, radius, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{"failed to query nearby"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

func (s *Server) createSubscription(w http.ResponseWriter, r *http.Request) {
	var req subscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{"invalid payload"})
		return
	}
	if req.CourierID == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{"courier_id required"})
		return
	}
	if req.Topic == "" {
		req.Topic = s.cfg.FCMTopicPrefix + req.CourierID
	}
	sub, err := s.store.CreateSubscription(r.Context(), postgres.Subscription{CourierID: req.CourierID, Topic: req.Topic})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{"failed to create subscription"})
		return
	}
	writeJSON(w, http.StatusCreated, sub)
}

func (s *Server) deleteSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.store.DeleteSubscription(r.Context(), id); err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{"subscription not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) listSubscriptions(w http.ResponseWriter, r *http.Request) {
	courierID := r.URL.Query().Get("courier_id")
	if courierID == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{"courier_id required"})
		return
	}
	items, err := s.store.ListSubscriptions(r.Context(), courierID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{"failed to list subscriptions"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func parseIntDefault(val string, def int) int {
	if val == "" {
		return def
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return def
	}
	return parsed
}

func parseTimeDefault(val string, def time.Time) time.Time {
	if val == "" {
		return def
	}
	parsed, err := time.Parse(time.RFC3339Nano, val)
	if err != nil {
		return def
	}
	return parsed
}
