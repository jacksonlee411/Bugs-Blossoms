package middleware

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/gorilla/mux"

	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/routing"
)

type opsGuard struct {
	conf       *configuration.Configuration
	classifier *routing.Classifier
	cidrs      []netip.Prefix
}

func OpsGuard(conf *configuration.Configuration, entrypoint string) mux.MiddlewareFunc {
	if conf == nil {
		conf = configuration.Use()
	}
	rules, err := routing.LoadAllowlist("", entrypoint)
	if err != nil {
		rules = nil
	}
	g := &opsGuard{
		conf:       conf,
		classifier: routing.NewClassifier(rules),
		cidrs:      parseCIDRs(conf.OpsGuardCIDRs),
	}
	return g.middleware
}

func (g *opsGuard) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if g.conf.GoAppEnvironment != configuration.Production || !g.conf.OpsGuardEnabled {
			next.ServeHTTP(w, r)
			return
		}

		if g.classifier.ClassifyPath(r.URL.Path) != routing.RouteClassOps {
			next.ServeHTTP(w, r)
			return
		}

		if g.authorized(r) {
			next.ServeHTTP(w, r)
			return
		}

		http.NotFound(w, r)
	})
}

func (g *opsGuard) authorized(r *http.Request) bool {
	if len(g.cidrs) > 0 {
		if ip, ok := realIP(r, g.conf.RealIPHeader); ok {
			addr, err := netip.ParseAddr(ip)
			if err == nil {
				for _, p := range g.cidrs {
					if p.Contains(addr) {
						return true
					}
				}
			}
		}
	}

	if token := strings.TrimSpace(g.conf.OpsGuardToken); token != "" {
		if subtle.ConstantTimeCompare([]byte(tokenFromRequest(r)), []byte(token)) == 1 {
			return true
		}
	}

	if user := strings.TrimSpace(g.conf.OpsGuardBasicAuthUser); user != "" || strings.TrimSpace(g.conf.OpsGuardBasicAuthPass) != "" {
		u, p, ok := r.BasicAuth()
		if ok &&
			subtle.ConstantTimeCompare([]byte(u), []byte(g.conf.OpsGuardBasicAuthUser)) == 1 &&
			subtle.ConstantTimeCompare([]byte(p), []byte(g.conf.OpsGuardBasicAuthPass)) == 1 {
			return true
		}
	}

	return false
}

func parseCIDRs(raw string) []netip.Prefix {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t' })
	out := make([]netip.Prefix, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if p, err := netip.ParsePrefix(part); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func tokenFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if t := strings.TrimSpace(r.Header.Get("X-Ops-Token")); t != "" {
		return t
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[len("bearer "):])
	}
	return ""
}

func realIP(r *http.Request, header string) (string, bool) {
	if r == nil {
		return "", false
	}
	if header != "" {
		if v := strings.TrimSpace(r.Header.Get(header)); v != "" {
			// X-Forwarded-For style: take the first item
			if i := strings.IndexByte(v, ','); i >= 0 {
				v = strings.TrimSpace(v[:i])
			}
			return stripPort(v)
		}
	}
	return stripPort(r.RemoteAddr)
}

func stripPort(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	if host, _, err := net.SplitHostPort(s); err == nil {
		return host, true
	}
	return s, true
}
