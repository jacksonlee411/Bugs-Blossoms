package authz

import (
	"context"
	"strings"
)

// MissingPolicy captures a denied object/action combination for the current subject/domain.
type MissingPolicy struct {
	Domain string `json:"domain"`
	Object string `json:"object"`
	Action string `json:"action"`
}

// PolicySuggestion describes a single allow rule that can fix a missing policy.
type PolicySuggestion struct {
	Subject string `json:"subject"`
	Domain  string `json:"domain"`
	Object  string `json:"object"`
	Action  string `json:"action"`
	Effect  string `json:"effect"`
}

// ViewState exposes authorization information to presentation layers.
type ViewState struct {
	Subject         string            `json:"subject"`
	Tenant          string            `json:"tenant"`
	Capabilities    map[string]bool   `json:"capabilities"`
	MissingPolicies []MissingPolicy   `json:"missingPolicies"`
	meta            map[string]string // reserved for future metadata (e.g. PolicyInspector params)
}

// NewViewState builds a ViewState for a subject/tenant pair.
func NewViewState(subject, tenant string) *ViewState {
	return &ViewState{
		Subject:      subject,
		Tenant:       tenant,
		Capabilities: map[string]bool{},
		meta:         map[string]string{},
	}
}

// SetCapability stores a boolean flag (e.g. "core.users.list") for later template use.
func (v *ViewState) SetCapability(name string, allowed bool) {
	if v == nil {
		return
	}
	key := normalizeCapabilityKey(name)
	v.Capabilities[key] = allowed
}

// Capability reports whether a capability was previously recorded as allowed.
func (v *ViewState) Capability(name string) bool {
	allowed, ok := v.CapabilityValue(name)
	return ok && allowed
}

// CapabilityValue returns the stored capability flag and whether it exists.
func (v *ViewState) CapabilityValue(name string) (bool, bool) {
	if v == nil {
		return false, false
	}
	key := normalizeCapabilityKey(name)
	allowed, ok := v.Capabilities[key]
	return allowed, ok
}

func normalizeCapabilityKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// AddMissingPolicy appends a denied policy combination for unauthorized responses.
func (v *ViewState) AddMissingPolicy(policy MissingPolicy) {
	if v == nil {
		return
	}
	v.MissingPolicies = append(v.MissingPolicies, policy)
}

// SuggestDiff builds a policy suggestion that can be fed into policy draft flows.
func (v *ViewState) SuggestDiff(policy MissingPolicy) []PolicySuggestion {
	if v == nil {
		return nil
	}
	return []PolicySuggestion{{
		Subject: v.Subject,
		Domain:  policy.Domain,
		Object:  policy.Object,
		Action:  policy.Action,
		Effect:  "allow",
	}}
}

type viewStateContextKey struct{}

// WithViewState stores the provided ViewState in the context.
func WithViewState(ctx context.Context, state *ViewState) context.Context {
	if state == nil {
		return ctx
	}
	return context.WithValue(ctx, viewStateContextKey{}, state)
}

// ViewStateFromContext retrieves the ViewState if present.
func ViewStateFromContext(ctx context.Context) *ViewState {
	if ctx == nil {
		return nil
	}
	if state, ok := ctx.Value(viewStateContextKey{}).(*ViewState); ok {
		return state
	}
	return nil
}
