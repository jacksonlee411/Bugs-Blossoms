package authz

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	debugRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "authz",
		Subsystem: "debug",
		Name:      "requests_total",
		Help:      "Total number of Authz debug evaluations broken down by mode and result.",
	}, []string{"mode", "result"})

	debugLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "authz",
		Subsystem: "debug",
		Name:      "latency_seconds",
		Help:      "Latency distribution for Authz debug evaluations.",
		Buckets: []float64{
			0.0005, 0.001, 0.002, 0.005,
			0.01, 0.02, 0.05, 0.1,
			0.2, 0.5, 1, 2,
		},
	}, []string{"mode", "result"})
)

func recordDebugMetrics(mode Mode, allowed bool, latency time.Duration) {
	result := "denied"
	if allowed {
		result = "allowed"
	}
	labels := prometheus.Labels{
		"mode":   string(mode),
		"result": result,
	}
	debugRequests.With(labels).Inc()
	debugLatency.With(labels).Observe(latency.Seconds())
}
