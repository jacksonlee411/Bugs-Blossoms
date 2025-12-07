package controllers

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"

	authz "github.com/iota-uz/iota-sdk/pkg/authz"

	"github.com/iota-uz/iota-sdk/modules/core/presentation/controllers/dtos"
)

var sharedPolicyStageStore = newPolicyStageStore()

func usePolicyStageStore() *policyStageStore {
	return sharedPolicyStageStore
}

type policyStageStore struct {
	mu   sync.Mutex
	data map[string][]dtos.StagedPolicyEntry
}

func newPolicyStageStore() *policyStageStore {
	return &policyStageStore{
		data: make(map[string][]dtos.StagedPolicyEntry),
	}
}

func policyStageKey(userID uint, tenantID uuid.UUID) string {
	return fmt.Sprintf("%d:%s", userID, tenantID.String())
}

func (s *policyStageStore) Add(key string, payload dtos.StagePolicyRequest) ([]dtos.StagedPolicyEntry, error) {
	if strings.TrimSpace(payload.Type) == "" {
		return nil, errors.New("type is required")
	}
	if strings.TrimSpace(payload.Object) == "" {
		return nil, errors.New("object is required")
	}
	if strings.TrimSpace(payload.Action) == "" {
		return nil, errors.New("action is required")
	}
	if strings.TrimSpace(payload.Effect) == "" {
		return nil, errors.New("effect is required")
	}
	if strings.TrimSpace(payload.Domain) == "" {
		return nil, errors.New("domain is required")
	}

	stageKind := strings.ToLower(strings.TrimSpace(payload.StageKind))
	if stageKind == "" {
		stageKind = "add"
	}
	if stageKind != "add" && stageKind != "remove" {
		return nil, errors.New("stage_kind must be add or remove")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.data[key]
	if len(entries) >= 50 {
		return nil, errors.New("stage limit reached (50)")
	}
	id := uuid.New().String()
	entry := dtos.StagedPolicyEntry{
		ID:        id,
		StageKind: stageKind,
		PolicyEntryResponse: dtos.PolicyEntryResponse{
			Type:    payload.Type,
			Subject: payload.Subject,
			Domain:  payload.Domain,
			Object:  payload.Object,
			Action:  authz.NormalizeAction(payload.Action),
			Effect:  payload.Effect,
		},
	}
	entries = append(entries, entry)
	s.data[key] = entries
	return entries, nil
}

func (s *policyStageStore) Delete(key string, id string) ([]dtos.StagedPolicyEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, ok := s.data[key]
	if !ok {
		return []dtos.StagedPolicyEntry{}, nil
	}
	next := make([]dtos.StagedPolicyEntry, 0, len(entries))
	found := false
	for _, entry := range entries {
		if entry.ID == id {
			found = true
			continue
		}
		next = append(next, entry)
	}
	if !found {
		return nil, errors.New("stage entry not found")
	}
	s.data[key] = next
	return next, nil
}

func (s *policyStageStore) Clear(key string, subject, domain string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, ok := s.data[key]
	if !ok || len(entries) == 0 {
		return 0
	}
	if strings.TrimSpace(subject) == "" && strings.TrimSpace(domain) == "" {
		delete(s.data, key)
		return 0
	}
	next := make([]dtos.StagedPolicyEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Subject == subject && entry.Domain == domain {
			continue
		}
		next = append(next, entry)
	}
	if len(next) == 0 {
		delete(s.data, key)
		return 0
	}
	s.data[key] = next
	return len(next)
}

func (s *policyStageStore) List(key string, subject, domain string) []dtos.StagedPolicyEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.data[key]
	if len(entries) == 0 {
		return []dtos.StagedPolicyEntry{}
	}
	result := make([]dtos.StagedPolicyEntry, 0, len(entries))
	for _, entry := range entries {
		if subject != "" && entry.Subject != subject {
			continue
		}
		if domain != "" && entry.Domain != domain {
			continue
		}
		result = append(result, entry)
	}
	return result
}
