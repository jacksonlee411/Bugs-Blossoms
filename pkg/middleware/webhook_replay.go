package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/iota-uz/iota-sdk/pkg/routing"
)

type WebhookReplayProtectionOptions struct {
	Entrypoint    string
	AllowlistPath string

	TTL          time.Duration
	MaxBodyBytes int64
}

type webhookReplayProtector struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func newWebhookReplayProtector() *webhookReplayProtector {
	return &webhookReplayProtector{
		seen: make(map[string]time.Time),
	}
}

func (p *webhookReplayProtector) isSeen(key string, now time.Time) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	for k, exp := range p.seen {
		if !now.Before(exp) {
			delete(p.seen, k)
		}
	}

	exp, ok := p.seen[key]
	if !ok {
		return false
	}
	return now.Before(exp)
}

func (p *webhookReplayProtector) mark(key string, exp time.Time) {
	p.mu.Lock()
	p.seen[key] = exp
	p.mu.Unlock()
}

type statusCaptureWriter struct {
	http.ResponseWriter
	statusCode    int
	statusWritten bool
}

func (w *statusCaptureWriter) WriteHeader(code int) {
	if !w.statusWritten {
		w.statusCode = code
		w.statusWritten = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusCaptureWriter) Write(p []byte) (int, error) {
	if !w.statusWritten {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

func (w *statusCaptureWriter) Status() int {
	if w.statusCode == 0 {
		return http.StatusOK
	}
	return w.statusCode
}

// WebhookReplayProtection provides best-effort replay protection for webhook routes:
// - only activates for paths classified as `webhook` via routing allowlist;
// - considers a request a replay iff an identical (path + body) request previously returned 2xx within the TTL window.
func WebhookReplayProtection(opts WebhookReplayProtectionOptions) mux.MiddlewareFunc {
	if opts.TTL <= 0 {
		opts.TTL = 10 * time.Minute
	}
	if opts.MaxBodyBytes <= 0 {
		opts.MaxBodyBytes = 1 << 20 // 1 MiB
	}

	rules, err := routing.LoadAllowlist(opts.AllowlistPath, opts.Entrypoint)
	if err != nil {
		return func(next http.Handler) http.Handler { return next }
	}
	classifier := routing.NewClassifier(rules)
	protector := newWebhookReplayProtector()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r == nil || r.URL == nil {
				next.ServeHTTP(w, r)
				return
			}
			if r.Method != http.MethodPost {
				next.ServeHTTP(w, r)
				return
			}
			if classifier.ClassifyPath(r.URL.Path) != routing.RouteClassWebhook {
				next.ServeHTTP(w, r)
				return
			}

			var body []byte
			if r.Body != nil {
				limited := io.LimitReader(r.Body, opts.MaxBodyBytes+1)
				b, readErr := io.ReadAll(limited)
				if readErr != nil {
					http.Error(w, "failed to read request body", http.StatusBadRequest)
					return
				}
				if int64(len(b)) > opts.MaxBodyBytes {
					http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
					return
				}
				body = b
				r.Body = io.NopCloser(bytes.NewReader(body))
			}

			sum := sha256.Sum256(append([]byte(r.URL.Path+"\n"), body...))
			key := hex.EncodeToString(sum[:])

			now := time.Now()
			if protector.isSeen(key, now) {
				w.WriteHeader(http.StatusOK)
				return
			}

			captured := &statusCaptureWriter{ResponseWriter: w}
			next.ServeHTTP(captured, r)

			status := captured.Status()
			if status >= 200 && status < 300 {
				protector.mark(key, now.Add(opts.TTL))
			}
		})
	}
}
