package services

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

// EmployeesAuthzObject represents the HRM employees capability object.
const EmployeesAuthzObject = "hrm.employees"
const PositionsAuthzObject = "hrm.positions"
const hrmAuthzDomain = "hrm"

var authorizeHRMFn = defaultAuthorizeHRM

func authorizeHRM(ctx context.Context, object, action string, opts ...authz.RequestOption) error {
	return authorizeHRMFn(ctx, object, action, opts...)
}

func defaultAuthorizeHRM(ctx context.Context, object, action string, opts ...authz.RequestOption) error {
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		tenantID = uuid.Nil
	}

	if subject, ok := authzutil.SystemSubjectFromContext(ctx); ok {
		req := authz.NewRequest(
			subject,
			hrmAuthzDomain,
			object,
			authz.NormalizeAction(action),
			opts...,
		)
		return authz.Use().Authorize(ctx, req)
	}

	currentUser, err := composables.UseUser(ctx)
	if err != nil {
		if errors.Is(err, composables.ErrNoUserFound) {
			return nil
		}
		return err
	}
	if currentUser == nil {
		return nil
	}

	req := authz.NewRequest(
		authzutil.SubjectForUser(tenantID, currentUser),
		hrmAuthzDomain,
		object,
		authz.NormalizeAction(action),
		opts...,
	)
	return authz.Use().Authorize(ctx, req)
}
