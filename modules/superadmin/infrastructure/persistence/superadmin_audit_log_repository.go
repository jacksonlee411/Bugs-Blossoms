package persistence

import (
	"context"

	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/modules/superadmin/domain"
	"github.com/iota-uz/iota-sdk/modules/superadmin/domain/entities"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/pkg/errors"
)

type pgSuperadminAuditLogRepository struct{}

func NewPgSuperadminAuditLogRepository() domain.SuperadminAuditLogRepository {
	return &pgSuperadminAuditLogRepository{}
}

func (r *pgSuperadminAuditLogRepository) ListByTenantID(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]*entities.SuperadminAuditLog, int, error) {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return nil, 0, errors.Wrap(err, "failed to get transaction")
	}

	var total int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM superadmin_audit_logs WHERE tenant_id = $1`, tenantID).Scan(&total); err != nil {
		return nil, 0, errors.Wrap(err, "failed to count audit logs")
	}

	rows, err := tx.Query(ctx, `
		SELECT
			id,
			actor_user_id,
			actor_email_snapshot,
			actor_name_snapshot,
			tenant_id,
			action,
			payload,
			ip_address::text,
			user_agent,
			created_at
		FROM superadmin_audit_logs
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, errors.Wrap(err, "failed to query audit logs")
	}
	defer rows.Close()

	var out []*entities.SuperadminAuditLog
	for rows.Next() {
		var a entities.SuperadminAuditLog
		if err := rows.Scan(
			&a.ID,
			&a.ActorUserID,
			&a.ActorEmail,
			&a.ActorName,
			&a.TenantID,
			&a.Action,
			&a.Payload,
			&a.IPAddress,
			&a.UserAgent,
			&a.CreatedAt,
		); err != nil {
			return nil, 0, errors.Wrap(err, "failed to scan audit log")
		}
		out = append(out, &a)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, errors.Wrap(err, "error iterating audit logs")
	}
	return out, total, nil
}
