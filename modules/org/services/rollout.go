package services

import (
	"strings"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

func OrgRolloutEnabledForTenant(tenantID uuid.UUID) bool {
	if tenantID == uuid.Nil {
		return false
	}

	cfg := configuration.Use()
	if cfg.OrgRolloutMode != "enabled" {
		return false
	}

	raw := strings.TrimSpace(cfg.OrgRolloutTenants)
	if raw == "" {
		return false
	}

	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t'
	}) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := uuid.Parse(part)
		if err != nil {
			continue
		}
		if id == tenantID {
			return true
		}
	}

	return false
}

func OrgCacheEnabled() bool {
	return configuration.Use().OrgCacheEnabled
}

func OrgReadStrategy() string {
	return configuration.Use().OrgReadStrategy
}
