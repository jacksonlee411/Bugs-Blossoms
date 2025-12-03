package persistence

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-faster/errors"
	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/repo"
	"github.com/jackc/pgx/v5"
)

const policyChangeRequestsTable = "policy_change_requests"

var (
	// ErrPolicyChangeRequestNotFound indicates that a record does not exist.
	ErrPolicyChangeRequestNotFound = errors.New("policy change request not found")
)

// PolicyChangeRequestRepository defines persistence operations.
type PolicyChangeRequestRepository interface {
	Create(ctx context.Context, req *PolicyChangeRequest) error
	GetByID(ctx context.Context, id uuid.UUID) (*PolicyChangeRequest, error)
	List(ctx context.Context, params FindParams) ([]PolicyChangeRequest, int64, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, params UpdateStatusParams) error
	UpdateBotMetadata(ctx context.Context, id uuid.UUID, params UpdateBotMetadataParams) error
	AcquireBotLock(ctx context.Context, id uuid.UUID, params BotLockParams) (bool, error)
	ReleaseBotLock(ctx context.Context, id uuid.UUID, locker string) error
	ForceReleaseBotLock(ctx context.Context, id uuid.UUID) error
}

type pgPolicyChangeRequestRepository struct{}

// NewPolicyChangeRequestRepository constructs a Postgres-backed repository.
func NewPolicyChangeRequestRepository() PolicyChangeRequestRepository {
	return &pgPolicyChangeRequestRepository{}
}

func (r *pgPolicyChangeRequestRepository) Create(ctx context.Context, req *PolicyChangeRequest) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	req.CreatedAt = now
	req.UpdatedAt = now
	if req.Diff == nil {
		req.Diff = jsonRawEmpty()
	}

	fields := []string{
		"status",
		"requester_id",
		"approver_id",
		"tenant_id",
		"subject",
		"domain",
		"action",
		"object",
		"reason",
		"diff",
		"base_policy_revision",
		"applied_policy_revision",
		"applied_policy_snapshot",
		"pr_link",
		"bot_job_id",
		"bot_lock",
		"bot_locked_at",
		"bot_attempts",
		"error_log",
		"created_at",
		"updated_at",
		"reviewed_at",
	}

	args := []interface{}{
		req.Status,
		req.RequesterID,
		nullableUUIDArg(req.ApproverID),
		req.TenantID,
		req.Subject,
		req.Domain,
		req.Action,
		req.Object,
		repoStringOrNull(req.Reason),
		[]byte(req.Diff),
		req.BasePolicyRevision,
		repoStringPointer(req.AppliedPolicyRevision),
		bytesOrNull(req.AppliedPolicySnapshot),
		repoStringPointer(req.PRLink),
		repoStringPointer(req.BotJobID),
		repoStringPointer(req.BotLock),
		timePointer(req.BotLockedAt),
		req.BotAttempts,
		repoStringPointer(req.ErrorLog),
		req.CreatedAt,
		req.UpdatedAt,
		timePointer(req.ReviewedAt),
	}

	query := repo.Insert(policyChangeRequestsTable, fields, "id")
	if err := tx.QueryRow(ctx, query, args...).Scan(&req.ID); err != nil {
		return errors.Wrap(err, "insert policy_change_requests")
	}
	return nil
}

func (r *pgPolicyChangeRequestRepository) GetByID(ctx context.Context, id uuid.UUID) (*PolicyChangeRequest, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	query := repo.Join("SELECT", policyChangeRequestColumns(), "FROM", policyChangeRequestsTable, "WHERE id = $1")
	row := tx.QueryRow(ctx, query, id)
	req, err := scanPolicyChangeRequest(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPolicyChangeRequestNotFound
		}
		return nil, err
	}
	return req, nil
}

func (r *pgPolicyChangeRequestRepository) List(ctx context.Context, params FindParams) ([]PolicyChangeRequest, int64, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, 0, err
	}

	where, args := buildFilters(params)
	selectQuery := repo.Join(
		"SELECT", policyChangeRequestColumns(),
		"FROM", policyChangeRequestsTable,
		repo.JoinWhere(where...),
		orderClause(params),
		repo.FormatLimitOffset(params.Limit, params.Offset),
	)

	rows, err := tx.Query(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, errors.Wrap(err, "list policy change requests")
	}
	defer rows.Close()

	var (
		results []PolicyChangeRequest
	)
	for rows.Next() {
		req, err := scanPolicyChangeRequest(rows)
		if err != nil {
			return nil, 0, err
		}
		results = append(results, *req)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	countQuery := repo.Join("SELECT COUNT(1) FROM", policyChangeRequestsTable, repo.JoinWhere(where...))
	var total int64
	if err := tx.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, errors.Wrap(err, "count policy change requests")
	}

	return results, total, nil
}

func (r *pgPolicyChangeRequestRepository) UpdateStatus(ctx context.Context, id uuid.UUID, params UpdateStatusParams) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	fields := []string{"status", "updated_at"}
	args := []interface{}{params.Status, now}

	if !params.ReviewedAt.IsUnset() {
		fields = append(fields, "reviewed_at")
		args = append(args, nullableTimeArg(params.ReviewedAt))
	}
	if !params.ApproverID.IsUnset() {
		fields = append(fields, "approver_id")
		args = append(args, nullableUUIDArgNullable(params.ApproverID))
	}

	query := repo.Update(policyChangeRequestsTable, fields, fmt.Sprintf("id = $%d", len(fields)+1))
	args = append(args, id)

	tag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "update policy change request status")
	}
	if tag.RowsAffected() == 0 {
		return ErrPolicyChangeRequestNotFound
	}
	return nil
}

func (r *pgPolicyChangeRequestRepository) UpdateBotMetadata(ctx context.Context, id uuid.UUID, params UpdateBotMetadataParams) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}

	fields := make([]string, 0, 8)
	args := make([]interface{}, 0, 8)

	addField := func(column string, value interface{}) {
		fields = append(fields, column)
		args = append(args, value)
	}

	if !params.BotJobID.IsUnset() {
		addField("bot_job_id", nullableStringArg(params.BotJobID))
	}
	if !params.BotAttempts.IsUnset() {
		addField("bot_attempts", nullableIntArg(params.BotAttempts))
	}
	if !params.ErrorLog.IsUnset() {
		addField("error_log", nullableStringArg(params.ErrorLog))
	}
	if !params.PRLink.IsUnset() {
		addField("pr_link", nullableStringArg(params.PRLink))
	}
	if !params.AppliedPolicyRevision.IsUnset() {
		addField("applied_policy_revision", nullableStringArg(params.AppliedPolicyRevision))
	}
	if !params.AppliedPolicySnapshot.IsUnset() {
		addField("applied_policy_snapshot", nullableJSONArg(params.AppliedPolicySnapshot))
	}

	if len(fields) == 0 {
		return nil
	}

	addField("updated_at", time.Now().UTC())

	query := repo.Update(policyChangeRequestsTable, fields, fmt.Sprintf("id = $%d", len(fields)+1))
	args = append(args, id)

	tag, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "update bot metadata")
	}
	if tag.RowsAffected() == 0 {
		return ErrPolicyChangeRequestNotFound
	}
	return nil
}

func (r *pgPolicyChangeRequestRepository) AcquireBotLock(ctx context.Context, id uuid.UUID, params BotLockParams) (bool, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return false, err
	}

	query := `
		UPDATE policy_change_requests
		SET bot_lock = $2, bot_locked_at = $3, updated_at = $3
		WHERE id = $1
		  AND (
				bot_lock IS NULL
				OR bot_lock = ''
				OR bot_locked_at IS NULL
				OR bot_locked_at < $4
				OR bot_lock = $2
		  )
	`
	res, err := tx.Exec(ctx, query, id, params.Locker, params.LockedAt, params.StaleBefore)
	if err != nil {
		return false, errors.Wrap(err, "acquire bot lock")
	}
	return res.RowsAffected() > 0, nil
}

func (r *pgPolicyChangeRequestRepository) ReleaseBotLock(ctx context.Context, id uuid.UUID, locker string) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	query := `
		UPDATE policy_change_requests
		SET bot_lock = NULL, bot_locked_at = NULL, updated_at = $3
		WHERE id = $1 AND bot_lock = $2
	`
	res, err := tx.Exec(ctx, query, id, locker, time.Now().UTC())
	if err != nil {
		return errors.Wrap(err, "release bot lock")
	}
	if res.RowsAffected() == 0 {
		return ErrPolicyChangeRequestNotFound
	}
	return nil
}

func (r *pgPolicyChangeRequestRepository) ForceReleaseBotLock(ctx context.Context, id uuid.UUID) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	query := `
		UPDATE policy_change_requests
		SET bot_lock = NULL, bot_locked_at = NULL, updated_at = $2
		WHERE id = $1
	`
	res, err := tx.Exec(ctx, query, id, time.Now().UTC())
	if err != nil {
		return errors.Wrap(err, "force release bot lock")
	}
	if res.RowsAffected() == 0 {
		return ErrPolicyChangeRequestNotFound
	}
	return nil
}

func policyChangeRequestColumns() string {
	return strings.Join([]string{
		"id",
		"status",
		"requester_id",
		"approver_id",
		"tenant_id",
		"subject",
		"domain",
		"action",
		"object",
		"reason",
		"diff",
		"base_policy_revision",
		"applied_policy_revision",
		"applied_policy_snapshot",
		"pr_link",
		"bot_job_id",
		"bot_lock",
		"bot_locked_at",
		"bot_attempts",
		"error_log",
		"created_at",
		"updated_at",
		"reviewed_at",
	}, ", ")
}

func scanPolicyChangeRequest(row pgx.Row) (*PolicyChangeRequest, error) {
	var (
		req         PolicyChangeRequest
		approver    sql.NullString
		appliedRev  sql.NullString
		appliedSnap []byte
		prLink      sql.NullString
		botJob      sql.NullString
		botLock     sql.NullString
		botLockedAt sql.NullTime
		errorLog    sql.NullString
		reviewedAt  sql.NullTime
		reason      sql.NullString
	)

	err := row.Scan(
		&req.ID,
		&req.Status,
		&req.RequesterID,
		&approver,
		&req.TenantID,
		&req.Subject,
		&req.Domain,
		&req.Action,
		&req.Object,
		&reason,
		&req.Diff,
		&req.BasePolicyRevision,
		&appliedRev,
		&appliedSnap,
		&prLink,
		&botJob,
		&botLock,
		&botLockedAt,
		&req.BotAttempts,
		&errorLog,
		&req.CreatedAt,
		&req.UpdatedAt,
		&reviewedAt,
	)
	if err != nil {
		return nil, errors.Wrap(err, "scan policy change request")
	}

	req.ApproverID = uuidFromNullString(approver)
	req.Reason = reason.String
	req.AppliedPolicyRevision = stringFromNull(appliedRev)
	if len(appliedSnap) > 0 {
		req.AppliedPolicySnapshot = json.RawMessage(appliedSnap)
	}
	req.PRLink = stringFromNull(prLink)
	req.BotJobID = stringFromNull(botJob)
	req.BotLock = stringFromNull(botLock)
	req.BotLockedAt = timeFromNull(botLockedAt)
	req.ErrorLog = stringFromNull(errorLog)
	req.ReviewedAt = timeFromNull(reviewedAt)

	return &req, nil
}

func buildFilters(params FindParams) ([]string, []interface{}) {
	conditions := []string{"1 = 1"}
	args := []interface{}{}

	addCondition := func(cond string, value interface{}) {
		conditions = append(conditions, fmt.Sprintf(cond, len(args)+1))
		args = append(args, value)
	}

	if len(params.Statuses) > 0 {
		statuses := make([]string, len(params.Statuses))
		for i, s := range params.Statuses {
			statuses[i] = string(s)
		}
		addCondition("status = ANY($%d)", statuses)
	}
	if params.TenantID != nil {
		addCondition("tenant_id = $%d", *params.TenantID)
	}
	if params.RequesterID != nil {
		addCondition("requester_id = $%d", *params.RequesterID)
	}
	if params.ApproverID != nil {
		addCondition("approver_id = $%d", *params.ApproverID)
	}
	if params.Subject != "" {
		addCondition("subject ILIKE $%d", "%"+params.Subject+"%")
	}
	if params.Domain != "" {
		addCondition("domain = $%d", params.Domain)
	}

	return conditions, args
}

func orderClause(params FindParams) string {
	order := "DESC"
	if params.SortAsc {
		order = "ASC"
	}
	return fmt.Sprintf("ORDER BY updated_at %s", order)
}

func jsonRawEmpty() json.RawMessage {
	return json.RawMessage("[]")
}

func nullableUUIDArgNullable(n Nullable[uuid.UUID]) interface{} {
	if !n.Set {
		return nil
	}
	if n.Valid {
		return n.Value
	}
	return nil
}

func nullableTimeArg(n Nullable[time.Time]) interface{} {
	if !n.Set {
		return nil
	}
	if n.Valid {
		return n.Value
	}
	return nil
}

func repoStringOrNull(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

func repoStringPointer(value *string) interface{} {
	if value == nil {
		return nil
	}
	if *value == "" {
		return nil
	}
	return *value
}

func nullableStringArg(n Nullable[string]) interface{} {
	if !n.Set {
		return nil
	}
	if n.Valid {
		return n.Value
	}
	return nil
}

func nullableIntArg(n Nullable[int]) interface{} {
	if !n.Set {
		return nil
	}
	if n.Valid {
		return n.Value
	}
	return nil
}

func nullableJSONArg(n Nullable[json.RawMessage]) interface{} {
	if !n.Set {
		return nil
	}
	if n.Valid {
		return []byte(n.Value)
	}
	return nil
}

func nullableUUIDArg(value *uuid.UUID) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func timePointer(value *time.Time) interface{} {
	if value == nil {
		return nil
	}
	return *value
}

func bytesOrNull(value json.RawMessage) interface{} {
	if len(value) == 0 {
		return nil
	}
	return []byte(value)
}

func uuidFromNullString(value sql.NullString) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	parsed, err := uuid.Parse(value.String)
	if err != nil {
		return nil
	}
	return &parsed
}

func stringFromNull(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	v := value.String
	return &v
}

func timeFromNull(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}
