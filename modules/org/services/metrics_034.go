package services

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	orgCacheRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "org",
		Subsystem: "cache",
		Name:      "requests_total",
		Help:      "Total number of Org cache lookups broken down by cache and hit/miss.",
	}, []string{"cache", "result"})

	orgCacheInvalidate = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "org",
		Subsystem: "cache",
		Name:      "invalidate_total",
		Help:      "Total number of Org cache invalidations broken down by reason.",
	}, []string{"reason"})

	orgWriteConflicts = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "org",
		Subsystem: "write",
		Name:      "conflicts_total",
		Help:      "Total number of Org write conflicts broken down by kind.",
	}, []string{"kind"})

	orgDeepReadActiveBackend = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "org",
		Subsystem: "deep_read",
		Name:      "active_backend",
		Help:      "Selected deep-read backend (value is always 1 per backend label).",
	}, []string{"backend"})
)

func recordCacheRequest(cache string, hit bool) {
	result := "miss"
	if hit {
		result = "hit"
	}
	orgCacheRequests.WithLabelValues(cache, result).Inc()
}

func recordCacheInvalidate(reason string) {
	if reason == "" {
		reason = "manual"
	}
	orgCacheInvalidate.WithLabelValues(reason).Inc()
}

func recordWriteConflict(kind string) {
	if kind == "" {
		kind = "other"
	}
	orgWriteConflicts.WithLabelValues(kind).Inc()
}

// RecordDeepReadBackendMetric exposes the currently selected deep-read backend as a gauge label.
func RecordDeepReadBackendMetric(backend DeepReadBackend) {
	orgDeepReadActiveBackend.WithLabelValues(string(backend)).Set(1)
}
