package authz

import (
	"context"
	"fmt"
	"time"
)

// InspectionResult captures the full outcome of an authorization evaluation.
type InspectionResult struct {
	Allowed         bool
	Mode            Mode
	Trace           []string
	Latency         time.Duration
	OriginalRequest Request
}

// Inspect evaluates a request and returns diagnostic information for debugging.
func (s *Service) Inspect(ctx context.Context, req Request) (InspectionResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	start := time.Now()
	allowed, trace, err := s.enforcer.EnforceEx(req.Subject, req.Domain, req.Object, req.Action, req.Attributes)
	latency := time.Since(start)
	if err != nil {
		return InspectionResult{}, fmt.Errorf("authz: inspect failed: %w", err)
	}

	result := InspectionResult{
		Allowed: allowed,
		Mode:    s.Mode(),
		Trace:   append([]string{}, trace...),
		Latency: latency,
		OriginalRequest: Request{
			Subject:    req.Subject,
			Domain:     req.Domain,
			Object:     req.Object,
			Action:     req.Action,
			Attributes: cloneAttributes(req.Attributes),
		},
	}
	recordDebugMetrics(result.Mode, result.Allowed, latency)
	return result, nil
}
