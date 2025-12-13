package controllers

import (
	"sort"
	"strings"

	"github.com/iota-uz/iota-sdk/modules/core/presentation/controllers/dtos"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/viewmodels"
)

func summarizeStagedEntries(entries []dtos.StagedPolicyEntry) viewmodels.AuthzChangesSummary {
	summary := viewmodels.AuthzChangesSummary{}
	if len(entries) == 0 {
		return summary
	}

	resources := map[string]struct{}{}
	for _, entry := range entries {
		if strings.EqualFold(strings.TrimSpace(entry.StageKind), "remove") {
			summary.Removed++
		} else {
			summary.Added++
		}
		if value := strings.TrimSpace(entry.Object); value != "" {
			resources[value] = struct{}{}
		}
	}

	if len(resources) == 0 {
		return summary
	}
	summary.Resources = make([]string, 0, len(resources))
	for value := range resources {
		summary.Resources = append(summary.Resources, value)
	}
	sort.Strings(summary.Resources)
	return summary
}

