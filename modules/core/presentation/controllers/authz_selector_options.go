package controllers

import (
	"sort"
	"strings"

	"github.com/iota-uz/iota-sdk/modules/core/services"
)

type authzSelectorOptions struct {
	Objects []string
	Actions []string
	Roles   []string
}

func buildAuthzSelectorOptions(entries []services.PolicyEntry) authzSelectorOptions {
	objects := map[string]struct{}{}
	actions := map[string]struct{}{}
	roles := map[string]struct{}{}

	for _, entry := range entries {
		typ := strings.ToLower(strings.TrimSpace(entry.Type))
		switch typ {
		case "g", "g2":
			if strings.HasPrefix(entry.Object, "role:") {
				roles[entry.Object] = struct{}{}
			}
			if strings.HasPrefix(entry.Subject, "role:") {
				roles[entry.Subject] = struct{}{}
			}
		default:
			obj := strings.TrimSpace(entry.Object)
			act := strings.TrimSpace(entry.Action)
			if obj != "" {
				objects[obj] = struct{}{}
			}
			if act != "" {
				actions[act] = struct{}{}
			}
			if strings.HasPrefix(entry.Subject, "role:") {
				roles[entry.Subject] = struct{}{}
			}
		}
	}

	ensureActions(actions, []string{"*", "view", "list", "read", "create", "update", "delete"})

	return authzSelectorOptions{
		Objects: sortedKeys(objects),
		Actions: sortedKeys(actions),
		Roles:   sortedKeys(roles),
	}
}

func ensureActions(actions map[string]struct{}, values []string) {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		actions[value] = struct{}{}
	}
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
