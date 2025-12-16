package seed

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/modules/core/domain/entities/tenant"
	"github.com/iota-uz/iota-sdk/modules/core/infrastructure/persistence"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

func CreateDefaultTenant(ctx context.Context, app application.Application) error {
	conf := configuration.Use()
	logger := conf.Logger()
	tenantRepository := persistence.NewTenantRepository()
	defaultTenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	desiredDomain := "default.localhost"

	// Create a new tenant with a fixed UUID for the default tenant
	defaultTenant := tenant.New(
		"Default",
		tenant.WithID(defaultTenantID),
		tenant.WithDomain(desiredDomain),
	)

	existing, err := tenantRepository.GetByID(ctx, defaultTenantID)
	if err == nil && existing != nil {
		if conf.GoAppEnvironment != configuration.Production {
			current := strings.ToLower(strings.TrimSpace(existing.Domain()))
			if current != desiredDomain {
				existing.SetDomain(desiredDomain)
				if _, err := tenantRepository.Update(ctx, existing); err != nil {
					logger.Errorf("Failed to update default tenant domain: %v", err)
					return err
				}
				logger.Infof("Updated default tenant domain to %s", desiredDomain)
			}
		}
		if err := ensureTenantPrimaryDomain(ctx, defaultTenantID, desiredDomain); err != nil {
			logger.Errorf("Failed to ensure default tenant primary domain: %v", err)
			return err
		}
		logger.Infof("Default tenant already exists")
		return nil
	}

	logger.Infof("Creating default tenant")
	if _, err := tenantRepository.Create(ctx, defaultTenant); err != nil {
		logger.Errorf("Failed to create default tenant: %v", err)
		return err
	}
	if err := ensureTenantPrimaryDomain(ctx, defaultTenantID, desiredDomain); err != nil {
		logger.Errorf("Failed to ensure default tenant primary domain: %v", err)
		return err
	}
	return nil
}

func ensureTenantPrimaryDomain(ctx context.Context, tenantID uuid.UUID, domain string) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}
	hostname := strings.ToLower(strings.TrimSpace(domain))
	if hostname == "" {
		return nil
	}

	if _, err := tx.Exec(ctx, `
		UPDATE tenant_domains
		SET is_primary = false, updated_at = now()
		WHERE tenant_id = $1 AND is_primary = true
	`, tenantID); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, `
			INSERT INTO tenant_domains (tenant_id, hostname, is_primary, verification_token, verified_at, created_at, updated_at)
			VALUES ($1, $2, true, replace(gen_random_uuid()::text, '-', '') || replace(gen_random_uuid()::text, '-', ''), now(), now(), now())
			ON CONFLICT (hostname) DO UPDATE
			SET
				is_primary = true,
				verified_at = COALESCE(tenant_domains.verified_at, now()),
			updated_at = now()
		WHERE tenant_domains.tenant_id = EXCLUDED.tenant_id
	`, tenantID, hostname)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("tenant domain hostname already bound to another tenant: %s", hostname)
	}
	return nil
}
