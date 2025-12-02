package legacy

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Snapshot captures relational data needed to migrate legacy permissions.
type Snapshot struct {
	Permissions     map[uuid.UUID]Permission
	Roles           map[int64]Role
	RolePermissions map[int64][]uuid.UUID
	Users           map[int64]User
	UserRoles       map[int64][]int64
	UserPermissions map[int64][]uuid.UUID
}

type Permission struct {
	ID       uuid.UUID
	Name     string
	Resource string
	Action   string
}

type Role struct {
	ID       int64
	TenantID uuid.UUID
	Name     string
}

type User struct {
	ID       int64
	TenantID uuid.UUID
}

// LoadSnapshot pulls the minimal set of legacy records needed for export/parity tools.
func LoadSnapshot(ctx context.Context, pool *pgxpool.Pool) (*Snapshot, error) {
	s := &Snapshot{
		Permissions:     map[uuid.UUID]Permission{},
		Roles:           map[int64]Role{},
		RolePermissions: map[int64][]uuid.UUID{},
		Users:           map[int64]User{},
		UserRoles:       map[int64][]int64{},
		UserPermissions: map[int64][]uuid.UUID{},
	}

	if err := s.loadPermissions(ctx, pool); err != nil {
		return nil, err
	}
	if err := s.loadRoles(ctx, pool); err != nil {
		return nil, err
	}
	if err := s.loadRolePermissions(ctx, pool); err != nil {
		return nil, err
	}
	if err := s.loadUsers(ctx, pool); err != nil {
		return nil, err
	}
	if err := s.loadUserRoles(ctx, pool); err != nil {
		return nil, err
	}
	if err := s.loadUserPermissions(ctx, pool); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Snapshot) loadPermissions(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `SELECT id, name, resource, action FROM permissions`)
	if err != nil {
		return fmt.Errorf("load permissions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.ID, &p.Name, &p.Resource, &p.Action); err != nil {
			return fmt.Errorf("scan permission: %w", err)
		}
		s.Permissions[p.ID] = p
	}
	return rows.Err()
}

func (s *Snapshot) loadRoles(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `SELECT id, COALESCE(tenant_id::text, '00000000-0000-0000-0000-000000000000') AS tenant_id, name FROM roles`)
	if err != nil {
		return fmt.Errorf("load roles: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			roleID   int64
			tenantID string
			name     string
		)
		if err := rows.Scan(&roleID, &tenantID, &name); err != nil {
			return fmt.Errorf("scan role: %w", err)
		}
		tenantUUID, err := uuid.Parse(tenantID)
		if err != nil {
			return fmt.Errorf("parse role tenant uuid: %w", err)
		}
		s.Roles[roleID] = Role{
			ID:       roleID,
			TenantID: tenantUUID,
			Name:     name,
		}
	}
	return rows.Err()
}

func (s *Snapshot) loadRolePermissions(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `SELECT role_id, permission_id FROM role_permissions`)
	if err != nil {
		return fmt.Errorf("load role_permissions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			roleID       int64
			permissionID uuid.UUID
		)
		if err := rows.Scan(&roleID, &permissionID); err != nil {
			return fmt.Errorf("scan role_permission: %w", err)
		}
		s.RolePermissions[roleID] = append(s.RolePermissions[roleID], permissionID)
	}
	return rows.Err()
}

func (s *Snapshot) loadUsers(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `SELECT id, COALESCE(tenant_id::text, '00000000-0000-0000-0000-000000000000') AS tenant_id FROM users`)
	if err != nil {
		return fmt.Errorf("load users: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			userID   int64
			tenantID string
		)
		if err := rows.Scan(&userID, &tenantID); err != nil {
			return fmt.Errorf("scan user: %w", err)
		}
		tenantUUID, err := uuid.Parse(tenantID)
		if err != nil {
			return fmt.Errorf("parse user tenant uuid: %w", err)
		}
		s.Users[userID] = User{
			ID:       userID,
			TenantID: tenantUUID,
		}
	}
	return rows.Err()
}

func (s *Snapshot) loadUserRoles(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `SELECT user_id, role_id FROM user_roles`)
	if err != nil {
		return fmt.Errorf("load user_roles: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var userID, roleID int64
		if err := rows.Scan(&userID, &roleID); err != nil {
			return fmt.Errorf("scan user_role: %w", err)
		}
		s.UserRoles[userID] = append(s.UserRoles[userID], roleID)
	}
	return rows.Err()
}

func (s *Snapshot) loadUserPermissions(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `SELECT user_id, permission_id FROM user_permissions`)
	if err != nil {
		return fmt.Errorf("load user_permissions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			userID       int64
			permissionID uuid.UUID
		)
		if err := rows.Scan(&userID, &permissionID); err != nil {
			return fmt.Errorf("scan user_permission: %w", err)
		}
		s.UserPermissions[userID] = append(s.UserPermissions[userID], permissionID)
	}
	return rows.Err()
}
