package authz

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	globalDomain          = "global"
	subjectTenantPrefix   = "tenant"
	subjectUserPrefix     = "user"
	rolePrefix            = "role"
	objectSeparator       = "."
	subjectSeparator      = ":"
	defaultActionWildcard = "*"
)

// Attributes contain optional ABAC style attributes supplied with a request.
type Attributes map[string]any

// Request encapsulates all parameters required to evaluate a Casbin rule.
type Request struct {
	Subject    string
	Domain     string
	Object     string
	Action     string
	Attributes Attributes
}

// RequestOption mutates a Request.
type RequestOption func(*Request)

// WithAttributes assigns attributes to the enforcement request.
func WithAttributes(attrs Attributes) RequestOption {
	return func(r *Request) {
		r.Attributes = attrs
	}
}

// NewRequest constructs a Request with sane defaults.
func NewRequest(subject, domain, object, action string, opts ...RequestOption) Request {
	req := Request{
		Subject:    subject,
		Domain:     domain,
		Object:     object,
		Action:     action,
		Attributes: Attributes{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&req)
		}
	}
	return req
}

// SubjectForUser builds a subject identifier in the form tenant:{tenantID}:user:{userID}.
func SubjectForUser(tenantID, userID uuid.UUID) string {
	userPart := "anonymous"
	if userID != uuid.Nil {
		userPart = userID.String()
	}
	return SubjectForUserID(tenantID, userPart)
}

// SubjectForUserID builds a subject identifier using a custom user identifier value.
func SubjectForUserID(tenantID uuid.UUID, userID string) string {
	tenantPart := DomainFromTenant(tenantID)
	userPart := strings.TrimSpace(userID)
	if userPart == "" {
		userPart = "anonymous"
	}
	builder := strings.Builder{}
	builder.Grow(len(subjectTenantPrefix) + len(subjectUserPrefix) + len(tenantPart) + len(userPart) + 4)
	builder.WriteString(subjectTenantPrefix)
	builder.WriteString(subjectSeparator)
	builder.WriteString(tenantPart)
	builder.WriteString(subjectSeparator)
	builder.WriteString(subjectUserPrefix)
	builder.WriteString(subjectSeparator)
	builder.WriteString(userPart)
	return builder.String()
}

// SubjectForRole returns the canonical identifier for a role-based subject.
func SubjectForRole(roleSlug string) string {
	roleSlug = strings.TrimSpace(roleSlug)
	if roleSlug == "" {
		roleSlug = "unnamed"
	}
	if strings.HasPrefix(roleSlug, rolePrefix+subjectSeparator) {
		return roleSlug
	}
	return fmt.Sprintf("%s%s%s", rolePrefix, subjectSeparator, strings.ToLower(roleSlug))
}

// DomainFromTenant converts a tenant ID into a casbin domain string.
func DomainFromTenant(id uuid.UUID) string {
	if id == uuid.Nil {
		return globalDomain
	}
	return strings.ToLower(id.String())
}

// ObjectName returns the canonical module.resource string, lowercased.
func ObjectName(module, resource string) string {
	module = strings.ToLower(strings.TrimSpace(module))
	resource = strings.ToLower(strings.TrimSpace(resource))
	if module == "" {
		module = "global"
	}
	if resource == "" {
		resource = "resource"
	}
	return module + objectSeparator + resource
}

// NormalizeAction returns a normalized action string.
func NormalizeAction(action string) string {
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		return defaultActionWildcard
	}
	return action
}
