package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/logging/domain/entities/authenticationlog"
	"github.com/iota-uz/iota-sdk/modules/logging/infrastructure/persistence/models"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/repo"
)

type AuthenticationLogRepository struct{}

func NewAuthenticationLogRepository() authenticationlog.Repository {
	return &AuthenticationLogRepository{}
}

func (r *AuthenticationLogRepository) List(
	ctx context.Context,
	params *authenticationlog.FindParams,
) ([]*authenticationlog.AuthenticationLog, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, err
	}
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return nil, err
	}

	where, args := buildAuthLogFilters(params, tenantID)
	query := `
		SELECT id, tenant_id, user_id, ip, user_agent, created_at
		FROM authentication_logs
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

	var results []*authenticationlog.AuthenticationLog
	for rows.Next() {
		var row models.AuthenticationLog
		if err := rows.Scan(&row.ID, &row.TenantID, &row.UserID, &row.IP, &row.UserAgent, &row.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, toDomainAuthenticationLog(&row))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (r *AuthenticationLogRepository) Count(ctx context.Context, params *authenticationlog.FindParams) (int64, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return 0, err
	}
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return 0, err
	}
	where, args := buildAuthLogFilters(params, tenantID)

	var count int64
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM authentication_logs
		WHERE `+strings.Join(where, " AND "),
		args...,
	).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *AuthenticationLogRepository) Create(ctx context.Context, log *authenticationlog.AuthenticationLog) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return err
	}

	dbRow := toDBAuthenticationLog(log)
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
		`INSERT INTO authentication_logs (tenant_id, user_id, ip, user_agent, created_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at`,
		dbRow.TenantID,
		dbRow.UserID,
		dbRow.IP,
		dbRow.UserAgent,
		dbRow.CreatedAt,
	).Scan(&log.ID, &log.CreatedAt)
}

func buildAuthLogFilters(params *authenticationlog.FindParams, tenantID uuid.UUID) ([]string, []interface{}) {
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
