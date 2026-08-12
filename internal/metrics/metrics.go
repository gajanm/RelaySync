package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	IngestRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ingest_requests_total",
			Help: "Total ingest requests",
		},
		[]string{"status"},
	)
	IngestLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "ingest_latency_seconds",
			Help:    "Latency for ingest endpoint",
			Buckets: prometheus.DefBuckets,
		},
	)
	RedisWriteLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "redis_write_latency_seconds",
			Help:    "Latency for Redis writes",
			Buckets: prometheus.DefBuckets,
		},
	)
	StreamLag = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "stream_lag",
			Help: "Approximate Redis stream lag",
		},
	)
	BatchInsertSize = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "batch_insert_size",
			Help:    "Batch size for insert",
			Buckets: []float64{1, 10, 50, 100, 200, 500},
		},
	)
	NotificationsSent = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notifications_sent_total",
			Help: "Total notifications sent",
		},
		[]string{"status"},
	)
	NotifierLatency = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "notifier_latency_seconds",
			Help:    "Notifier send latency",
			Buckets: prometheus.DefBuckets,
		},
	)
)

func Register(r *prometheus.Registry) {
	r.MustRegister(
		IngestRequestsTotal,
		IngestLatency,
		RedisWriteLatency,
		StreamLag,
		BatchInsertSize,
		NotificationsSent,
		NotifierLatency,
	)
}
