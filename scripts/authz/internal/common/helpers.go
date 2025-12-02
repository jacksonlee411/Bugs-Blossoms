package common

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/pkg/authz"
)

var moduleHints = map[string]string{
	"user":     "core",
	"role":     "core",
	"group":    "core",
	"upload":   "core",
	"employee": "hrm",
}

var nonSlugChars = regexp.MustCompile(`[^a-z0-9]+`)

// ModuleForPermission attempts to derive the logical module name from a permission name/resource.
func ModuleForPermission(name, resource string) string {
	if name != "" {
		prefix := strings.ToLower(strings.SplitN(name, ".", 2)[0])
		if module, ok := moduleHints[prefix]; ok {
			return module
		}
	}
	resource = strings.ToLower(strings.TrimSpace(resource))
	if module, ok := moduleHints[resource]; ok {
		return module
	}
	if resource != "" {
		return resource
	}
	return "global"
}

// RoleSubject builds the canonical subject for a role.
func RoleSubject(tenantID uuid.UUID, roleName string) string {
	domain := authz.DomainFromTenant(tenantID)
	slug := slugify(roleName)
	return authz.SubjectForRole(domain + ":" + slug)
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = nonSlugChars.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "role"
	}
	return value
}

// HashID shortens sensitive identifiers for exports.
func HashID(value string, enabled bool) string {
	if !enabled {
		return value
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}
