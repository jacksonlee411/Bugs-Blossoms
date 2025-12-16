package seed

import (
	"context"
	"net"
	"strings"

	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/modules/core/domain/entities/tenant"
	"github.com/iota-uz/iota-sdk/modules/core/infrastructure/persistence"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

func CreateDefaultTenant(ctx context.Context, app application.Application) error {
	conf := configuration.Use()
	logger := conf.Logger()
	tenantRepository := persistence.NewTenantRepository()
	defaultTenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	desiredDomain := strings.ToLower(strings.TrimSpace(conf.Domain))
	if desiredDomain == "" {
		desiredDomain = "default.localhost"
	}
	if h, _, err := net.SplitHostPort(desiredDomain); err == nil {
		desiredDomain = strings.ToLower(strings.TrimSpace(h))
	}

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
		logger.Infof("Default tenant already exists")
		return nil
	}

	logger.Infof("Creating default tenant")
	if _, err := tenantRepository.Create(ctx, defaultTenant); err != nil {
		logger.Errorf("Failed to create default tenant: %v", err)
		return err
	}
	return nil
}
