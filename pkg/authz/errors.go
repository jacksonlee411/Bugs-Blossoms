package authz

import (
	"fmt"

	"github.com/iota-uz/iota-sdk/pkg/serrors"
)

const (
	errorCodeForbidden = "AUTHZ_FORBIDDEN"
	errorLocaleKey     = "Authorization.PermissionDenied"
)

// forbiddenError builds a standardized error for denied policies.
func forbiddenError(req Request) *serrors.BaseError {
	return serrors.NewError(
		errorCodeForbidden,
		"permission denied",
		errorLocaleKey,
	).WithTemplateData(map[string]string{
		"object":  req.Object,
		"action":  req.Action,
		"domain":  req.Domain,
		"subject": req.Subject,
	})
}

// configError standardizes configuration validation errors.
func configError(msg string, args ...any) error {
	return fmt.Errorf("authz: "+msg, args...)
}
