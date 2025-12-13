package controllers

import (
	"sort"
	"strings"

	"github.com/iota-uz/iota-sdk/modules/core/presentation/controllers/dtos"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/viewmodels"
)

func buildWorkspacePreview(entries []dtos.StagedPolicyEntry) viewmodels.AuthzWorkspacePreview {
	preview := viewmodels.AuthzWorkspacePreview{
		Items: make([]viewmodels.AuthzWorkspacePreviewItem, 0, len(entries)),
	}
	if len(entries) == 0 {
		return preview
	}

	domains := map[string]struct{}{}
	for _, entry := range entries {
		stageKind := strings.ToLower(strings.TrimSpace(entry.StageKind))
		if stageKind == "" {
			stageKind = "add"
		}
		typ := strings.ToLower(strings.TrimSpace(entry.Type))
		domain := strings.TrimSpace(entry.Domain)
		object := strings.TrimSpace(entry.Object)
		action := strings.TrimSpace(entry.Action)
		effect := strings.ToLower(strings.TrimSpace(entry.Effect))

		if domain != "" {
			domains[domain] = struct{}{}
		}

		isGrouping := typ == "g" || typ == "g2"
		if !isGrouping {
			if effect == "deny" {
				preview.DenyCount++
			}
			if strings.Contains(object, "*") || strings.Contains(action, "*") {
				preview.WildcardCount++
			}
		}

		preview.Items = append(preview.Items, viewmodels.AuthzWorkspacePreviewItem{
			StageKind: stageKind,
			Type:      typ,
			Domain:    domain,
			Object:    object,
			Action:    action,
			Effect:    effect,
		})
	}

	if len(domains) > 0 {
		preview.Domains = make([]string, 0, len(domains))
		for domain := range domains {
			preview.Domains = append(preview.Domains, domain)
		}
		sort.Strings(preview.Domains)
	}

	sort.SliceStable(preview.Items, func(i, j int) bool {
		left := preview.Items[i]
		right := preview.Items[j]
		if left.StageKind != right.StageKind {
			if left.StageKind == "remove" {
				return true
			}
			if right.StageKind == "remove" {
				return false
			}
			return left.StageKind < right.StageKind
		}
		if left.Type != right.Type {
			return left.Type < right.Type
		}
		if left.Domain != right.Domain {
			return left.Domain < right.Domain
		}
		if left.Object != right.Object {
			return left.Object < right.Object
		}
		if left.Action != right.Action {
			return left.Action < right.Action
		}
		return left.Effect < right.Effect
	})

	return preview
}
