package outbox

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type metrics struct {
	enqueueTotal  *prometheus.CounterVec
	dispatchTotal *prometheus.CounterVec
	deadTotal     *prometheus.CounterVec

	dispatchLatency *prometheus.HistogramVec

	pending     *prometheus.GaugeVec
	locked      *prometheus.GaugeVec
	relayLeader *prometheus.GaugeVec
}

var metricsSingleton = sync.OnceValue(func() *metrics {
	return &metrics{
		enqueueTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "outbox",
			Name:      "enqueue_total",
			Help:      "Total number of outbox enqueue operations.",
		}, []string{"table", "topic"}),
		dispatchTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "outbox",
			Name:      "dispatch_total",
			Help:      "Total number of outbox dispatch operations.",
		}, []string{"table", "topic", "result"}),
		deadTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "outbox",
			Name:      "dead_total",
			Help:      "Total number of messages that first entered dead state.",
		}, []string{"table", "topic"}),
		dispatchLatency: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "outbox",
			Name:      "dispatch_latency_seconds",
			Help:      "Latency distribution for outbox dispatch.",
			Buckets: []float64{
				0.001, 0.002, 0.005,
				0.01, 0.02, 0.05,
				0.1, 0.2, 0.5,
				1, 2, 5, 10,
			},
		}, []string{"table", "topic", "result"}),
		pending: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "outbox",
			Name:      "pending",
			Help:      "Current number of pending (unpublished) messages.",
		}, []string{"table"}),
		locked: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "outbox",
			Name:      "locked",
			Help:      "Current number of locked (unpublished) messages.",
		}, []string{"table"}),
		relayLeader: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "outbox",
			Name:      "relay_leader",
			Help:      "Whether current instance holds leader lock for a table (1/0).",
		}, []string{"table"}),
	}
})

func getMetrics() *metrics {
	return metricsSingleton()
}
