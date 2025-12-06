package services

import (
	"context"

	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/serrors"
)

const LogsAuthzObject = "logging.logs"

var authorizeLoggingFn = defaultAuthorizeLogging

func authorizeLogging(ctx context.Context, action string, opts ...authz.RequestOption) error {
	return authorizeLoggingFn(ctx, action, opts...)
}

func defaultAuthorizeLogging(ctx context.Context, action string, opts ...authz.RequestOption) error {
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return serrors.NewError("AUTHZ_FORBIDDEN", "tenant not found", "Authorization.PermissionDenied")
	}

	if subject, ok := authzutil.SystemSubjectFromContext(ctx); ok {
		req := authz.NewRequest(
			subject,
			authz.DomainFromTenant(tenantID),
			LogsAuthzObject,
			authz.NormalizeAction(action),
			opts...,
		)
		return authz.Use().Authorize(ctx, req)
	}

	currentUser, err := composables.UseUser(ctx)
	if err != nil || currentUser == nil {
		return serrors.NewError("AUTHZ_FORBIDDEN", "permission denied", "Authorization.PermissionDenied")
	}

	req := authz.NewRequest(
		authzutil.SubjectForUser(tenantID, currentUser),
		authz.DomainFromTenant(tenantID),
		LogsAuthzObject,
		authz.NormalizeAction(action),
		opts...,
	)
	return authz.Use().Authorize(ctx, req)
}
