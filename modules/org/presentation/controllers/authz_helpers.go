package controllers

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/core/authzutil"
	coreuser "github.com/iota-uz/iota-sdk/modules/core/domain/aggregates/user"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/serrors"
)

var (
	orgHierarchiesAuthzObject = authz.ObjectName("org", "hierarchies")
	orgNodesAuthzObject       = authz.ObjectName("org", "nodes")
	orgEdgesAuthzObject       = authz.ObjectName("org", "edges")
	orgAssignmentsAuthzObject = authz.ObjectName("org", "assignments")
	orgSnapshotAuthzObject    = authz.ObjectName("org", "snapshot")
	orgBatchAuthzObject       = authz.ObjectName("org", "batch")
	orgChangeRequestsAuthzObj = authz.ObjectName("org", "change_requests")
	orgPreflightAuthzObject   = authz.ObjectName("org", "preflight")
)

func ensureOrgAuthz(
	w http.ResponseWriter,
	r *http.Request,
	tenantID uuid.UUID,
	currentUser coreuser.User,
	object,
	action string,
	opts ...authz.RequestOption,
) bool {
	ctxWithState, state := authzutil.EnsureViewStateOrAnonymous(r.Context(), tenantID, currentUser)
	if ctxWithState != r.Context() {
		*r = *r.WithContext(ctxWithState)
	}

	if tenantID == uuid.Nil || currentUser == nil {
		writeForbiddenPayload(w, r, state, object, action)
		return false
	}

	svc := authz.Use()
	req := authz.NewRequest(
		authzutil.SubjectForUser(tenantID, currentUser),
		authzutil.DomainFromContext(r.Context()),
		object,
		authz.NormalizeAction(action),
		opts...,
	)

	if err := svc.Authorize(r.Context(), req); err != nil {
		var be *serrors.BaseError
		if errors.As(err, &be) && be.Code == "AUTHZ_FORBIDDEN" {
			recordMissingPolicy(state, r, object, action)
			writeForbiddenPayload(w, r, state, object, action)
			return false
		}
		writeAPIError(w, http.StatusInternalServerError, authzutil.RequestIDFromRequest(r), "ORG_INTERNAL", err.Error())
		return false
	}

	if svc.Mode() == authz.ModeShadow {
		allowed, err := svc.Check(r.Context(), req)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, authzutil.RequestIDFromRequest(r), "ORG_INTERNAL", err.Error())
			return false
		}
		if !allowed {
			recordMissingPolicy(state, r, object, action)
		}
	}

	return true
}

func recordMissingPolicy(state *authz.ViewState, r *http.Request, object, action string) {
	if state == nil {
		return
	}
	state.AddMissingPolicy(authz.MissingPolicy{
		Domain: authzutil.DomainFromContext(r.Context()),
		Object: object,
		Action: authz.NormalizeAction(action),
	})
}

func writeForbiddenPayload(w http.ResponseWriter, r *http.Request, state *authz.ViewState, object, action string) {
	payload := authzutil.BuildForbiddenPayload(r, state, object, action)
	writeJSON(w, http.StatusForbidden, payload)
}
