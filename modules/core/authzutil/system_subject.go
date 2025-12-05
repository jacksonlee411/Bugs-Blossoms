package authzutil

import (
	"context"
	"strings"
)

type systemSubjectKey struct{}

// WithSystemSubject stores a synthetic subject (e.g. system:core.job) in the context.
func WithSystemSubject(ctx context.Context, subject string) context.Context {
	subject = normalizeSystemSubject(subject)
	if subject == "" {
		return ctx
	}
	return context.WithValue(ctx, systemSubjectKey{}, subject)
}

// SystemSubjectFromContext extracts the synthetic subject if present.
func SystemSubjectFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	subject, ok := ctx.Value(systemSubjectKey{}).(string)
	if !ok || strings.TrimSpace(subject) == "" {
		return "", false
	}
	return subject, true
}

// SystemSubject builds a canonical subject identifier for internal actors (e.g. background jobs).
func SystemSubject(actor string) string {
	return normalizeSystemSubject(actor)
}

func normalizeSystemSubject(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return ""
	}
	if !strings.HasPrefix(subject, "system:") {
		subject = "system:" + subject
	}
	return strings.ToLower(subject)
}
