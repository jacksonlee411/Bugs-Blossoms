package controllers

import (
	"bufio"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	orgAPIRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "org",
		Subsystem: "api",
		Name:      "requests_total",
		Help:      "Total number of Org API requests broken down by endpoint and result.",
	}, []string{"endpoint", "result"})

	orgAPILatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "org",
		Subsystem: "api",
		Name:      "latency_seconds",
		Help:      "Latency distribution for Org API requests.",
		Buckets: []float64{
			0.001, 0.002, 0.005,
			0.01, 0.02, 0.05,
			0.1, 0.2, 0.5,
			1, 2, 5, 10,
		},
	}, []string{"endpoint", "result"})
)

type statusRecordingResponseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusRecordingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecordingResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusRecordingResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusRecordingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return h.Hijack()
}

func (w *statusRecordingResponseWriter) Push(target string, opts *http.PushOptions) error {
	p, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return p.Push(target, opts)
}

func (c *OrgAPIController) instrumentAPI(endpoint string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecordingResponseWriter{ResponseWriter: w, status: http.StatusOK}

		next(rec, r)

		result := "2xx"
		switch {
		case rec.status >= 500:
			result = "5xx"
		case rec.status >= 400:
			result = "4xx"
		}

		orgAPIRequests.WithLabelValues(endpoint, result).Inc()
		orgAPILatency.WithLabelValues(endpoint, result).Observe(time.Since(start).Seconds())
	}
}
