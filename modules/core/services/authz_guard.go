package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

func authorizeCore(ctx context.Context, object, action string, opts ...authz.RequestOption) error {
	currentUser, err := composables.UseUser(ctx)
	if err != nil || currentUser == nil {
		return nil
	}
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		tenantID = uuid.Nil
	}

	req := authz.NewRequest(
		authzutil.SubjectForUser(tenantID, currentUser),
		authz.DomainFromTenant(tenantID),
		object,
		authz.NormalizeAction(action),
		opts...,
	)
	return authz.Use().Authorize(ctx, req)
}
