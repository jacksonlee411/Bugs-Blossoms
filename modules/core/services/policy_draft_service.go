package services

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	authz "github.com/iota-uz/iota-sdk/pkg/authz"
	authzPersistence "github.com/iota-uz/iota-sdk/pkg/authz/persistence"
	authzVersion "github.com/iota-uz/iota-sdk/pkg/authz/version"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/eventbus"
)

var (
	// ErrPolicyDraftNotFound indicates the draft is missing or belongs to another tenant.
	ErrPolicyDraftNotFound = errors.New("policy draft not found")
	// ErrInvalidDiff indicates that the diff payload is empty or invalid JSON.
	ErrInvalidDiff = errors.New("policy draft diff must be valid JSON")
	// ErrRevisionMismatch indicates that the provided revision mismatches the current policy revision.
	ErrRevisionMismatch = errors.New("policy draft base revision is stale")
	// ErrInvalidStatusTransition indicates the requested transition is not allowed.
	ErrInvalidStatusTransition = errors.New("policy draft status transition not allowed")
	// ErrTenantMismatch indicates the draft does not belong to the scoped tenant.
	ErrTenantMismatch = errors.New("policy draft belongs to a different tenant")
	// ErrMissingSnapshot indicates that a revert was requested without an applied snapshot.
	ErrMissingSnapshot = errors.New("policy draft snapshot is not available")
)

// PolicyDraft represents a single policy change request.
type PolicyDraft struct {
	ID                    uuid.UUID
	Status                authzPersistence.PolicyChangeStatus
	RequesterID           uuid.UUID
	ApproverID            *uuid.UUID
	TenantID              uuid.UUID
	Subject               string
	Domain                string
	Action                string
	Object                string
	Reason                string
	Diff                  json.RawMessage
	BasePolicyRevision    string
	AppliedPolicyRevision *string
	AppliedPolicySnapshot json.RawMessage
	PRLink                *string
	BotJobID              *string
	BotLock               *string
	BotLockedAt           *time.Time
	BotAttempts           int
	ErrorLog              *string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	ReviewedAt            *time.Time
}

// PolicyDraftCreatedEvent is published whenever a draft is created.
type PolicyDraftCreatedEvent struct {
	Draft PolicyDraft
}

// PolicyDraftStatusChangedEvent is published whenever a draft status mutates.
type PolicyDraftStatusChangedEvent struct {
	PreviousStatus authzPersistence.PolicyChangeStatus
	Draft          PolicyDraft
}

// PolicyEntry describes a single row from config/access/policy.csv.
type PolicyEntry struct {
	Type    string `json:"type"`
	Subject string `json:"subject"`
	Domain  string `json:"domain"`
	Object  string `json:"object"`
	Action  string `json:"action"`
	Effect  string `json:"effect"`
}

// CreatePolicyDraftParams captures input fields required to create a draft.
type CreatePolicyDraftParams struct {
	RequesterID  uuid.UUID
	Object       string
	Action       string
	Reason       string
	Diff         json.RawMessage
	Domain       string
	Subject      string
	BaseRevision string
}

// ListPolicyDraftsParams controls pagination and filtering for draft lists.
type ListPolicyDraftsParams struct {
	Statuses    []authzPersistence.PolicyChangeStatus
	RequesterID *uuid.UUID
	ApproverID  *uuid.UUID
	Subject     string
	Domain      string
	Limit       int
	Offset      int
	SortAsc     bool
}

// PolicyDraftService orchestrates policy change requests.
type PolicyDraftService struct {
	repo             authzPersistence.PolicyChangeRequestRepository
	versionProvider  authzVersion.Provider
	policyPath       string
	publisher        eventbus.EventBus
	maxPageSize      int
	defaultPageSize  int
	staleLockTimeout time.Duration
}

// NewPolicyDraftService constructs a service instance.
func NewPolicyDraftService(
	repo authzPersistence.PolicyChangeRequestRepository,
	versionProvider authzVersion.Provider,
	policyPath string,
	publisher eventbus.EventBus,
) *PolicyDraftService {
	return &PolicyDraftService{
		repo:             repo,
		versionProvider:  versionProvider,
		policyPath:       policyPath,
		publisher:        publisher,
		maxPageSize:      500,
		defaultPageSize:  50,
		staleLockTimeout: time.Minute * 5,
	}
}

// Create registers a new policy change draft scoped to the provided tenant.
func (s *PolicyDraftService) Create(
	ctx context.Context,
	tenantID uuid.UUID,
	params CreatePolicyDraftParams,
) (PolicyDraft, error) {
	if params.RequesterID == uuid.Nil {
		return PolicyDraft{}, errors.New("policy draft requester is required")
	}
	if strings.TrimSpace(params.Object) == "" {
		return PolicyDraft{}, errors.New("policy draft object is required")
	}
	if strings.TrimSpace(params.Action) == "" {
		return PolicyDraft{}, errors.New("policy draft action is required")
	}

	diff, err := normalizeDiff(params.Diff)
	if err != nil {
		return PolicyDraft{}, err
	}

	meta, err := s.versionProvider.Current(ctx)
	if err != nil {
		return PolicyDraft{}, err
	}
	if params.BaseRevision != "" && params.BaseRevision != meta.Revision {
		return PolicyDraft{}, ErrRevisionMismatch
	}

	domain := params.Domain
	if strings.TrimSpace(domain) == "" {
		domain = authz.DomainFromTenant(tenantID)
	}
	subject := params.Subject
	if strings.TrimSpace(subject) == "" {
		subject = authz.SubjectForUser(tenantID, params.RequesterID)
	}

	req := &authzPersistence.PolicyChangeRequest{
		Status:             authzPersistence.PolicyChangeStatusPendingReview,
		RequesterID:        params.RequesterID,
		TenantID:           tenantID,
		Subject:            subject,
		Domain:             domain,
		Action:             authz.NormalizeAction(params.Action),
		Object:             strings.TrimSpace(params.Object),
		Reason:             strings.TrimSpace(params.Reason),
		Diff:               diff,
		BasePolicyRevision: meta.Revision,
	}

	err = composables.InTx(ctx, func(txCtx context.Context) error {
		return s.repo.Create(txCtx, req)
	})
	if err != nil {
		return PolicyDraft{}, err
	}

	draft := mapPolicyDraft(req)
	s.publisher.Publish(PolicyDraftCreatedEvent{Draft: draft})
	return draft, nil
}

// List returns drafts for a tenant with pagination.
func (s *PolicyDraftService) List(
	ctx context.Context,
	tenantID uuid.UUID,
	params ListPolicyDraftsParams,
) ([]PolicyDraft, int64, error) {
	listParams := authzPersistence.FindParams{
		TenantID: &tenantID,
		Statuses: params.Statuses,
		Subject:  params.Subject,
		Domain:   params.Domain,
		Limit:    clampLimit(params.Limit, s.defaultPageSize, s.maxPageSize),
		Offset:   maxInt(params.Offset, 0),
		SortAsc:  params.SortAsc,
	}
	if params.RequesterID != nil {
		listParams.RequesterID = params.RequesterID
	}
	if params.ApproverID != nil {
		listParams.ApproverID = params.ApproverID
	}

	results, total, err := s.repo.List(ctx, listParams)
	if err != nil {
		return nil, 0, err
	}
	drafts := make([]PolicyDraft, 0, len(results))
	for i := range results {
		drafts = append(drafts, mapPolicyDraft(&results[i]))
	}
	return drafts, total, nil
}

// Get returns a draft by ID if it belongs to the tenant.
func (s *PolicyDraftService) Get(
	ctx context.Context,
	tenantID uuid.UUID,
	id uuid.UUID,
) (PolicyDraft, error) {
	req, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, authzPersistence.ErrPolicyChangeRequestNotFound) {
			return PolicyDraft{}, ErrPolicyDraftNotFound
		}
		return PolicyDraft{}, err
	}
	if req.TenantID != tenantID {
		return PolicyDraft{}, ErrTenantMismatch
	}
	return mapPolicyDraft(req), nil
}

// Approve transitions a draft into the approved state.
func (s *PolicyDraftService) Approve(
	ctx context.Context,
	tenantID uuid.UUID,
	id uuid.UUID,
	approverID uuid.UUID,
) (PolicyDraft, error) {
	req, err := s.ensureOwned(ctx, tenantID, id)
	if err != nil {
		return PolicyDraft{}, err
	}
	if req.Status != authzPersistence.PolicyChangeStatusPendingReview {
		return PolicyDraft{}, ErrInvalidStatusTransition
	}
	now := time.Now().UTC()
	params := authzPersistence.UpdateStatusParams{
		Status:     authzPersistence.PolicyChangeStatusApproved,
		ApproverID: authzPersistence.NewNullableValue(approverID),
		ReviewedAt: authzPersistence.NewNullableValue(now),
	}
	if err := s.repo.UpdateStatus(ctx, id, params); err != nil {
		return PolicyDraft{}, err
	}
	updated, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return PolicyDraft{}, err
	}
	draft := mapPolicyDraft(updated)
	s.publisher.Publish(PolicyDraftStatusChangedEvent{
		PreviousStatus: req.Status,
		Draft:          draft,
	})
	return draft, nil
}

// Reject moves a draft into the rejected state.
func (s *PolicyDraftService) Reject(
	ctx context.Context,
	tenantID uuid.UUID,
	id uuid.UUID,
	approverID uuid.UUID,
) (PolicyDraft, error) {
	req, err := s.ensureOwned(ctx, tenantID, id)
	if err != nil {
		return PolicyDraft{}, err
	}
	if req.Status != authzPersistence.PolicyChangeStatusPendingReview {
		return PolicyDraft{}, ErrInvalidStatusTransition
	}
	now := time.Now().UTC()
	params := authzPersistence.UpdateStatusParams{
		Status:     authzPersistence.PolicyChangeStatusRejected,
		ApproverID: authzPersistence.NewNullableValue(approverID),
		ReviewedAt: authzPersistence.NewNullableValue(now),
	}
	if err := s.repo.UpdateStatus(ctx, id, params); err != nil {
		return PolicyDraft{}, err
	}
	updated, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return PolicyDraft{}, err
	}
	draft := mapPolicyDraft(updated)
	s.publisher.Publish(PolicyDraftStatusChangedEvent{
		PreviousStatus: req.Status,
		Draft:          draft,
	})
	return draft, nil
}

// Cancel allows a requestor to cancel a pending draft.
func (s *PolicyDraftService) Cancel(
	ctx context.Context,
	tenantID uuid.UUID,
	id uuid.UUID,
) (PolicyDraft, error) {
	req, err := s.ensureOwned(ctx, tenantID, id)
	if err != nil {
		return PolicyDraft{}, err
	}
	switch req.Status {
	case authzPersistence.PolicyChangeStatusDraft,
		authzPersistence.PolicyChangeStatusPendingReview,
		authzPersistence.PolicyChangeStatusApproved:
	case authzPersistence.PolicyChangeStatusRejected,
		authzPersistence.PolicyChangeStatusMerged,
		authzPersistence.PolicyChangeStatusFailed,
		authzPersistence.PolicyChangeStatusCanceled:
		return PolicyDraft{}, ErrInvalidStatusTransition
	}
	params := authzPersistence.UpdateStatusParams{
		Status:     authzPersistence.PolicyChangeStatusCanceled,
		ApproverID: authzPersistence.NewNullableNull[uuid.UUID](),
		ReviewedAt: authzPersistence.NewNullableValue(time.Now().UTC()),
	}
	if err := s.repo.UpdateStatus(ctx, id, params); err != nil {
		return PolicyDraft{}, err
	}
	updated, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return PolicyDraft{}, err
	}
	draft := mapPolicyDraft(updated)
	s.publisher.Publish(PolicyDraftStatusChangedEvent{
		PreviousStatus: req.Status,
		Draft:          draft,
	})
	return draft, nil
}

// TriggerBot records a manual bot trigger attempt.
func (s *PolicyDraftService) TriggerBot(
	ctx context.Context,
	tenantID uuid.UUID,
	id uuid.UUID,
	locker string,
) (PolicyDraft, error) {
	req, err := s.ensureOwned(ctx, tenantID, id)
	if err != nil {
		return PolicyDraft{}, err
	}
	params := authzPersistence.UpdateBotMetadataParams{
		BotAttempts: authzPersistence.NewNullableValue(req.BotAttempts + 1),
	}
	if strings.TrimSpace(locker) != "" {
		params.BotJobID = authzPersistence.NewNullableValue(locker)
	}
	params.ErrorLog = authzPersistence.NewNullableNull[string]()
	if err := s.repo.UpdateBotMetadata(ctx, id, params); err != nil {
		return PolicyDraft{}, err
	}
	updated, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return PolicyDraft{}, err
	}
	return mapPolicyDraft(updated), nil
}

// Revert creates a new draft using the applied snapshot of an existing request.
func (s *PolicyDraftService) Revert(
	ctx context.Context,
	tenantID uuid.UUID,
	sourceID uuid.UUID,
	requesterID uuid.UUID,
) (PolicyDraft, error) {
	req, err := s.ensureOwned(ctx, tenantID, sourceID)
	if err != nil {
		return PolicyDraft{}, err
	}
	if len(req.AppliedPolicySnapshot) == 0 {
		return PolicyDraft{}, ErrMissingSnapshot
	}
	return s.Create(ctx, tenantID, CreatePolicyDraftParams{
		RequesterID: requesterID,
		Object:      req.Object,
		Action:      req.Action,
		Reason:      fmt.Sprintf("Revert policy change %s", sourceID),
		Diff:        req.AppliedPolicySnapshot,
		Domain:      req.Domain,
	})
}

// Policies returns the parsed contents of the aggregated policy file.
func (s *PolicyDraftService) Policies(ctx context.Context) ([]PolicyEntry, error) {
	if s.policyPath == "" {
		return nil, errors.New("policy file path is not configured")
	}
	file, err := os.Open(s.policyPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	reader := bufio.NewReader(file)
	var entries []PolicyEntry
	lineNumber := 0
	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		if trimmed := strings.TrimSpace(line); trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			lineNumber++
			entry, parseErr := parsePolicyEntry(trimmed)
			if parseErr != nil {
				return nil, fmt.Errorf("policy file line %d: %w", lineNumber, parseErr)
			}
			entries = append(entries, entry)
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}

	return entries, nil
}

func (s *PolicyDraftService) ensureOwned(
	ctx context.Context,
	tenantID uuid.UUID,
	id uuid.UUID,
) (*authzPersistence.PolicyChangeRequest, error) {
	req, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, authzPersistence.ErrPolicyChangeRequestNotFound) {
			return nil, ErrPolicyDraftNotFound
		}
		return nil, err
	}
	if req.TenantID != tenantID {
		return nil, ErrTenantMismatch
	}
	return req, nil
}

func mapPolicyDraft(req *authzPersistence.PolicyChangeRequest) PolicyDraft {
	return PolicyDraft{
		ID:                    req.ID,
		Status:                req.Status,
		RequesterID:           req.RequesterID,
		ApproverID:            req.ApproverID,
		TenantID:              req.TenantID,
		Subject:               req.Subject,
		Domain:                req.Domain,
		Action:                req.Action,
		Object:                req.Object,
		Reason:                req.Reason,
		Diff:                  append(json.RawMessage(nil), req.Diff...),
		BasePolicyRevision:    req.BasePolicyRevision,
		AppliedPolicyRevision: req.AppliedPolicyRevision,
		AppliedPolicySnapshot: append(json.RawMessage(nil), req.AppliedPolicySnapshot...),
		PRLink:                req.PRLink,
		BotJobID:              req.BotJobID,
		BotLock:               req.BotLock,
		BotLockedAt:           req.BotLockedAt,
		BotAttempts:           req.BotAttempts,
		ErrorLog:              req.ErrorLog,
		CreatedAt:             req.CreatedAt,
		UpdatedAt:             req.UpdatedAt,
		ReviewedAt:            req.ReviewedAt,
	}
}

func normalizeDiff(data json.RawMessage) (json.RawMessage, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, ErrInvalidDiff
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, data); err != nil {
		return nil, ErrInvalidDiff
	}
	return json.RawMessage(buf.Bytes()), nil
}

func clampLimit(limit, def, maxAllowed int) int {
	switch {
	case limit <= 0:
		return def
	case limit > maxAllowed:
		return maxAllowed
	default:
		return limit
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func parsePolicyEntry(line string) (PolicyEntry, error) {
	reader := csv.NewReader(strings.NewReader(line))
	reader.TrimLeadingSpace = true
	record, err := reader.Read()
	if err != nil {
		return PolicyEntry{}, err
	}
	for i := range record {
		record[i] = strings.TrimSpace(record[i])
	}
	entry := PolicyEntry{}
	if len(record) == 0 {
		return entry, errors.New("empty policy row")
	}
	get := func(idx int) string {
		if idx < 0 || idx >= len(record) {
			return ""
		}
		return record[idx]
	}
	entry = PolicyEntry{
		Type:    get(0),
		Subject: get(1),
		Domain:  get(2),
		Object:  get(3),
		Action:  get(4),
		Effect:  "allow",
	}
	if eff := get(5); eff != "" {
		entry.Effect = eff
	}
	switch entry.Type {
	case "g", "g2":
		entry.Object = get(2)
		if domain := get(3); domain != "" {
			entry.Domain = domain
		}
		if entry.Action == "" {
			entry.Action = "*"
		}
	default:
		if entry.Action == "" {
			entry.Action = "*"
		}
	}
	return entry, nil
}
