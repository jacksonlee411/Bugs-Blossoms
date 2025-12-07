package controllers

import (
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/iota-uz/iota-sdk/modules/core/services"
)

type PolicyListParams struct {
	Subject   string
	Domain    string
	Type      string
	Search    string
	Page      int
	Limit     int
	SortField string
	SortAsc   bool
}

func (p PolicyListParams) Offset() int {
	if p.Page <= 1 {
		return 0
	}
	return (p.Page - 1) * p.Limit
}

func parsePolicyListParams(values url.Values) (PolicyListParams, error) {
	params := PolicyListParams{
		Page:      1,
		Limit:     50,
		SortField: "object",
		SortAsc:   true,
	}
	if subject := strings.TrimSpace(values.Get("subject")); subject != "" {
		params.Subject = subject
	}
	if domain := strings.TrimSpace(values.Get("domain")); domain != "" {
		params.Domain = domain
	}
	if typ := strings.TrimSpace(values.Get("type")); typ != "" {
		params.Type = typ
	}
	if search := strings.TrimSpace(values.Get("q")); search != "" {
		params.Search = search
	}
	if page := values.Get("page"); page != "" {
		val, err := strconv.Atoi(page)
		if err != nil || val < 1 {
			return params, errors.New("page must be a positive integer")
		}
		params.Page = val
	}
	if limit := values.Get("limit"); limit != "" {
		val, err := strconv.Atoi(limit)
		if err != nil || val < 1 {
			return params, errors.New("limit must be a positive integer")
		}
		if val > 500 {
			val = 500
		}
		params.Limit = val
	}
	if sort := values.Get("sort"); sort != "" {
		parts := strings.Split(sort, ":")
		field := strings.TrimSpace(parts[0])
		if field != "" {
			params.SortField = field
		}
		if len(parts) > 1 && strings.EqualFold(parts[1], "desc") {
			params.SortAsc = false
		}
	}
	return params, nil
}

func filterPolicies(entries []services.PolicyEntry, params PolicyListParams) []services.PolicyEntry {
	results := make([]services.PolicyEntry, 0, len(entries))
	for _, entry := range entries {
		if params.Subject != "" && entry.Subject != params.Subject {
			continue
		}
		if params.Domain != "" && entry.Domain != params.Domain {
			continue
		}
		if params.Type != "" && entry.Type != params.Type {
			continue
		}
		if params.Search != "" {
			search := strings.ToLower(params.Search)
			if !strings.Contains(strings.ToLower(entry.Object), search) &&
				!strings.Contains(strings.ToLower(entry.Action), search) {
				continue
			}
		}
		results = append(results, entry)
	}
	return results
}

func sortPolicies(entries []services.PolicyEntry, field string, asc bool) []services.PolicyEntry {
	less := func(i, j int) bool { return entries[i].Object < entries[j].Object }
	switch field {
	case "subject":
		less = func(i, j int) bool { return entries[i].Subject < entries[j].Subject }
	case "domain":
		less = func(i, j int) bool { return entries[i].Domain < entries[j].Domain }
	case "type":
		less = func(i, j int) bool { return entries[i].Type < entries[j].Type }
	case "action":
		less = func(i, j int) bool { return entries[i].Action < entries[j].Action }
	}
	sorted := make([]services.PolicyEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		if asc {
			return less(i, j)
		}
		return less(j, i)
	})
	return sorted
}

func paginatePolicies(entries []services.PolicyEntry, params PolicyListParams) ([]services.PolicyEntry, int) {
	filtered := filterPolicies(entries, params)
	sorted := sortPolicies(filtered, params.SortField, params.SortAsc)
	start := params.Offset()
	end := start + params.Limit
	if start > len(sorted) {
		start = len(sorted)
	}
	if end > len(sorted) {
		end = len(sorted)
	}
	return sorted[start:end], len(sorted)
}
