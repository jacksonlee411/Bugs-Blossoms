package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/actionlog"
	"github.com/iota-uz/iota-sdk/modules/logging/infrastructure/persistence/models"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/repo"
)

type ActionLogRepository struct{}

func NewActionLogRepository() actionlog.Repository {
	return &ActionLogRepository{}
}

func (r *ActionLogRepository) List(ctx context.Context, params *actionlog.FindParams) ([]*actionlog.ActionLog, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return nil, err
	}

	where, args := buildActionLogFilters(params, tenantID)
	query := `
		SELECT id, tenant_id, user_id, method, path, before, after, user_agent, ip, created_at
		FROM action_logs
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY created_at DESC
	`
	if params != nil {
		query += " " + repo.FormatLimitOffset(params.Limit, params.Offset)
	}

	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*actionlog.ActionLog
	for rows.Next() {
		var row models.ActionLog
		if err := rows.Scan(
			&row.ID,
			&row.TenantID,
			&row.UserID,
			&row.Method,
			&row.Path,
			&row.Before,
			&row.After,
			&row.UserAgent,
			&row.IP,
			&row.CreatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, toDomainActionLog(&row))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *ActionLogRepository) Count(ctx context.Context, params *actionlog.FindParams) (int64, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return 0, err
	}
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return 0, err
	}
	where, args := buildActionLogFilters(params, tenantID)

	var count int64
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM action_logs
		WHERE `+strings.Join(where, " AND "),
		args...,
	).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *ActionLogRepository) Create(ctx context.Context, log *actionlog.ActionLog) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return err
	}

	dbRow := toDBActionLog(log)
	if log != nil && log.TenantID == uuid.Nil {
		dbRow.TenantID = ""
	}
	if dbRow.TenantID == "" {
		dbRow.TenantID = tenantID.String()
	}
	if log != nil && log.TenantID == uuid.Nil {
		log.TenantID = tenantID
	}
	if dbRow.CreatedAt.IsZero() {
		dbRow.CreatedAt = time.Now()
	}

	return tx.QueryRow(
		ctx,
		`INSERT INTO action_logs (tenant_id, method, path, user_id, before, after, user_agent, ip, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING id, created_at`,
		dbRow.TenantID,
		dbRow.Method,
		dbRow.Path,
		dbRow.UserID,
		dbRow.Before,
		dbRow.After,
		dbRow.UserAgent,
		dbRow.IP,
		dbRow.CreatedAt,
	).Scan(&log.ID, &log.CreatedAt)
}

func buildActionLogFilters(params *actionlog.FindParams, tenantID uuid.UUID) ([]string, []interface{}) {
	where := []string{"tenant_id = $1"}
	args := []interface{}{tenantID}
	argPos := 2
	if params == nil {
		return where, args
	}

	if params.UserID != nil {
		where = append(where, fmt.Sprintf("user_id = $%d", argPos))
		args = append(args, *params.UserID)
		argPos++
	}
	if method := strings.TrimSpace(params.Method); method != "" {
		where = append(where, fmt.Sprintf("LOWER(method) = LOWER($%d)", argPos))
		args = append(args, method)
		argPos++
	}
	if path := strings.TrimSpace(params.Path); path != "" {
		where = append(where, fmt.Sprintf("path ILIKE $%d", argPos))
		args = append(args, "%"+path+"%")
		argPos++
	}
	if ip := strings.TrimSpace(params.IP); ip != "" {
		where = append(where, fmt.Sprintf("ip ILIKE $%d", argPos))
		args = append(args, "%"+ip+"%")
		argPos++
	}
	if ua := strings.TrimSpace(params.UserAgent); ua != "" {
		where = append(where, fmt.Sprintf("user_agent ILIKE $%d", argPos))
		args = append(args, "%"+ua+"%")
		argPos++
	}
	if params.From != nil && !params.From.IsZero() {
		where = append(where, fmt.Sprintf("created_at >= $%d", argPos))
		args = append(args, *params.From)
		argPos++
	}
	if params.To != nil && !params.To.IsZero() {
		where = append(where, fmt.Sprintf("created_at <= $%d", argPos))
		args = append(args, *params.To)
	}
	return where, args
}
