package controllers

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/iota-uz/iota-sdk/modules/core/presentation/templates/pages/users"
	"github.com/iota-uz/iota-sdk/modules/core/services"
)

func buildUserEffectivePoliciesProps(
	allEntries []services.PolicyEntry,
	userSubject string,
	defaultDomain string,
	params PolicyListParams,
	roleNameToID map[string]uint,
	baseURL string,
) users.UserEffectivePoliciesProps {
	domain := strings.TrimSpace(params.Domain)
	if domain == "" {
		domain = defaultDomain
	}
	page := params.Page
	if page < 1 {
		page = 1
	}
	limit := params.Limit
	if limit < 1 {
		limit = 20
	}

	roleChains := buildRoleChains(allEntries, userSubject, domain)

	type effectiveKey struct {
		Domain string
		Object string
		Action string
		Effect string
	}
	type effectiveAgg struct {
		Direct bool
		Roles  map[string]users.UserEffectiveRoleSource
	}

	aggs := make(map[effectiveKey]*effectiveAgg)
	for _, entry := range allEntries {
		if entry.Type != "p" {
			continue
		}
		effect := strings.ToLower(strings.TrimSpace(entry.Effect))
		if effect != "" && effect != "allow" {
			continue
		}
		if !policyDomainApplies(domain, entry.Domain) {
			continue
		}

		key := effectiveKey{
			Domain: strings.TrimSpace(entry.Domain),
			Object: strings.TrimSpace(entry.Object),
			Action: strings.TrimSpace(entry.Action),
			Effect: strings.TrimSpace(entry.Effect),
		}
		if key.Effect == "" {
			key.Effect = "allow"
		}

		agg, ok := aggs[key]
		if !ok {
			agg = &effectiveAgg{
				Roles: map[string]users.UserEffectiveRoleSource{},
			}
			aggs[key] = agg
		}

		subject := strings.TrimSpace(entry.Subject)
		switch subject {
		case userSubject:
			agg.Direct = true
		default:
			chain, ok := roleChains[subject]
			if !ok {
				continue
			}
			if _, exists := agg.Roles[subject]; exists {
				continue
			}
			roleName := strings.TrimPrefix(strings.TrimSpace(subject), "role:")
			rolePoliciesURL := ""
			if roleName != "" && roleNameToID != nil {
				if roleID, ok := roleNameToID[roleName]; ok && roleID > 0 {
					rolePoliciesURL = fmt.Sprintf("/roles/%d/policies?domain=%s", roleID, url.QueryEscape(domain))
				}
			}
			agg.Roles[subject] = users.UserEffectiveRoleSource{
				Subject:     subject,
				Chain:       chain,
				PoliciesURL: rolePoliciesURL,
			}
		}
	}

	effectiveList := make([]users.UserEffectivePolicyEntry, 0, len(aggs))
	for key, agg := range aggs {
		roleSources := make([]users.UserEffectiveRoleSource, 0, len(agg.Roles))
		for _, roleSource := range agg.Roles {
			roleSources = append(roleSources, roleSource)
		}
		sort.Slice(roleSources, func(i, j int) bool {
			return roleSources[i].Subject < roleSources[j].Subject
		})

		effectiveList = append(effectiveList, users.UserEffectivePolicyEntry{
			Domain: key.Domain,
			Object: key.Object,
			Action: key.Action,
			Effect: key.Effect,
			Direct: agg.Direct,
			Roles:  roleSources,
		})
	}

	search := strings.ToLower(strings.TrimSpace(params.Search))
	if search != "" {
		filtered := make([]users.UserEffectivePolicyEntry, 0, len(effectiveList))
		for _, entry := range effectiveList {
			if strings.Contains(strings.ToLower(entry.Domain), search) ||
				strings.Contains(strings.ToLower(entry.Object), search) ||
				strings.Contains(strings.ToLower(entry.Action), search) {
				filtered = append(filtered, entry)
				continue
			}
			matched := false
			for _, roleSource := range entry.Roles {
				if strings.Contains(strings.ToLower(roleSource.Subject), search) {
					matched = true
					break
				}
			}
			if matched {
				filtered = append(filtered, entry)
			}
		}
		effectiveList = filtered
	}

	sort.Slice(effectiveList, func(i, j int) bool {
		if effectiveList[i].Domain != effectiveList[j].Domain {
			return effectiveList[i].Domain < effectiveList[j].Domain
		}
		if effectiveList[i].Object != effectiveList[j].Object {
			return effectiveList[i].Object < effectiveList[j].Object
		}
		if effectiveList[i].Action != effectiveList[j].Action {
			return effectiveList[i].Action < effectiveList[j].Action
		}
		return effectiveList[i].Effect < effectiveList[j].Effect
	})

	total := len(effectiveList)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	return users.UserEffectivePoliciesProps{
		Entries:       effectiveList[start:end],
		Total:         total,
		Page:          page,
		Limit:         limit,
		DomainFilter:  domain,
		DefaultDomain: defaultDomain,
		Search:        strings.TrimSpace(params.Search),
		BaseURL:       baseURL,
	}
}

func policyDomainApplies(requestDomain, policyDomain string) bool {
	requestDomain = strings.TrimSpace(requestDomain)
	policyDomain = strings.TrimSpace(policyDomain)
	if requestDomain == "" {
		return false
	}
	if requestDomain == "global" {
		return true
	}
	return policyDomain == requestDomain || policyDomain == "*"
}

func buildRoleChains(
	allEntries []services.PolicyEntry,
	userSubject string,
	domain string,
) map[string][]string {
	domain = strings.TrimSpace(domain)

	edges := make(map[string][]string)
	for _, entry := range allEntries {
		if entry.Type != "g" {
			continue
		}
		if strings.TrimSpace(entry.Domain) != domain {
			continue
		}
		subject := strings.TrimSpace(entry.Subject)
		object := strings.TrimSpace(entry.Object)
		if subject == "" || object == "" {
			continue
		}
		edges[subject] = append(edges[subject], object)
	}
	for subject := range edges {
		sort.Strings(edges[subject])
	}

	chainByRole := make(map[string][]string)
	queue := make([]string, 0)
	for _, roleSubject := range edges[userSubject] {
		if roleSubject == "" {
			continue
		}
		if _, seen := chainByRole[roleSubject]; seen {
			continue
		}
		chainByRole[roleSubject] = []string{roleSubject}
		queue = append(queue, roleSubject)
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		chain := chainByRole[current]

		for _, next := range edges[current] {
			if next == "" {
				continue
			}
			if _, seen := chainByRole[next]; seen {
				continue
			}
			nextChain := make([]string, 0, len(chain)+1)
			nextChain = append(nextChain, chain...)
			nextChain = append(nextChain, next)
			chainByRole[next] = nextChain
			queue = append(queue, next)
		}
	}

	return chainByRole
}
