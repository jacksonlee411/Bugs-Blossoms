package services

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/modules/superadmin/domain"
	"github.com/iota-uz/iota-sdk/modules/superadmin/domain/entities"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/pkg/errors"
)

var (
	ErrTenantAuthInvalidIdentityMode  = errors.New("invalid identity mode")
	ErrTenantAuthNoLoginMethodEnabled = errors.New("at least one login method must be enabled")
	ErrTenantAuthSSORequiresEnabled   = errors.New("allow_sso requires at least one enabled and tested SSO connection")
)

type TenantAuthSettingsService struct {
	repo  domain.TenantAuthSettingsRepository
	sso   domain.TenantSSOConnectionsRepository
	audit *AuditService
}

func NewTenantAuthSettingsService(repo domain.TenantAuthSettingsRepository, ssoRepo domain.TenantSSOConnectionsRepository, audit *AuditService) *TenantAuthSettingsService {
	return &TenantAuthSettingsService{
		repo:  repo,
		sso:   ssoRepo,
		audit: audit,
	}
}

func (s *TenantAuthSettingsService) GetOrDefault(ctx context.Context, tenantID uuid.UUID) (*entities.TenantAuthSettings, error) {
	settings, err := s.repo.GetByTenantID(ctx, tenantID)
	if err != nil {
		if errors.Is(err, domain.ErrTenantAuthSettingsNotFound) {
			return &entities.TenantAuthSettings{
				TenantID:      tenantID,
				IdentityMode:  "legacy",
				AllowPassword: true,
				AllowGoogle:   true,
				AllowSSO:      false,
			}, nil
		}
		return nil, err
	}
	if settings == nil {
		return &entities.TenantAuthSettings{
			TenantID:      tenantID,
			IdentityMode:  "legacy",
			AllowPassword: true,
			AllowGoogle:   true,
			AllowSSO:      false,
		}, nil
	}
	return settings, nil
}

func (s *TenantAuthSettingsService) Update(ctx context.Context, tenantID uuid.UUID, identityMode string, allowPassword, allowGoogle, allowSSO bool) (*entities.TenantAuthSettings, error) {
	identityMode = strings.TrimSpace(strings.ToLower(identityMode))
	switch identityMode {
	case "legacy", "kratos":
	default:
		return nil, ErrTenantAuthInvalidIdentityMode
	}

	if !allowPassword && !allowGoogle && !allowSSO {
		return nil, ErrTenantAuthNoLoginMethodEnabled
	}

	if allowSSO {
		conns, err := s.sso.ListByTenantID(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		ok := false
		for _, c := range conns {
			if !c.Enabled {
				continue
			}
			if c.LastTestStatus != nil && *c.LastTestStatus == "ok" {
				ok = true
				break
			}
		}
		if !ok {
			return nil, ErrTenantAuthSSORequiresEnabled
		}
	}

	row := &entities.TenantAuthSettings{
		TenantID:      tenantID,
		IdentityMode:  identityMode,
		AllowPassword: allowPassword,
		AllowGoogle:   allowGoogle,
		AllowSSO:      allowSSO,
	}

	var updated *entities.TenantAuthSettings
	err := composables.InTx(composables.WithTenantID(ctx, tenantID), func(txCtx context.Context) error {
		res, err := s.repo.Upsert(txCtx, row)
		if err != nil {
			return err
		}
		updated = res
		if s.audit != nil {
			_ = s.audit.Log(txCtx, &tenantID, "tenant.auth.update", map[string]any{
				"settings": updated,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}
