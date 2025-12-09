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

func normalizeStageKind(stageKind string) (string, error) {
	kind := strings.ToLower(strings.TrimSpace(stageKind))
	if kind == "" {
		kind = "add"
	}
	if kind != "add" && kind != "remove" {
		return "", errors.New("stage_kind must be add or remove")
	}
	return kind, nil
}

func (s *policyStageStore) buildEntry(payload dtos.StagePolicyRequest) (dtos.StagedPolicyEntry, error) {
	if strings.TrimSpace(payload.Type) == "" {
		return dtos.StagedPolicyEntry{}, errors.New("type is required")
	}
	if strings.TrimSpace(payload.Object) == "" {
		return dtos.StagedPolicyEntry{}, errors.New("object is required")
	}
	if strings.TrimSpace(payload.Action) == "" {
		return dtos.StagedPolicyEntry{}, errors.New("action is required")
	}
	if strings.TrimSpace(payload.Effect) == "" {
		return dtos.StagedPolicyEntry{}, errors.New("effect is required")
	}
	if strings.TrimSpace(payload.Domain) == "" {
		return dtos.StagedPolicyEntry{}, errors.New("domain is required")
	}

	stageKind, err := normalizeStageKind(payload.StageKind)
	if err != nil {
		return dtos.StagedPolicyEntry{}, err
	}

	return dtos.StagedPolicyEntry{
		ID:        uuid.New().String(),
		StageKind: stageKind,
		PolicyEntryResponse: dtos.PolicyEntryResponse{
			Type:    payload.Type,
			Subject: payload.Subject,
			Domain:  payload.Domain,
			Object:  payload.Object,
			Action:  authz.NormalizeAction(payload.Action),
			Effect:  payload.Effect,
		},
	}, nil
}

func policyStageKey(userID uint, tenantID uuid.UUID) string {
	return fmt.Sprintf("%d:%s", userID, tenantID.String())
}

func (s *policyStageStore) Add(key string, payload dtos.StagePolicyRequest) ([]dtos.StagedPolicyEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.addAllLocked(key, []dtos.StagePolicyRequest{payload})
}

func (s *policyStageStore) AddMany(key string, payloads []dtos.StagePolicyRequest) ([]dtos.StagedPolicyEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.addAllLocked(key, payloads)
}

func (s *policyStageStore) addAllLocked(key string, payloads []dtos.StagePolicyRequest) ([]dtos.StagedPolicyEntry, error) {
	if len(payloads) == 0 {
		return nil, errors.New("at least one staged policy is required")
	}

	current := s.data[key]
	if len(current)+len(payloads) > 50 {
		return nil, errors.New("stage limit reached (50)")
	}

	entries := make([]dtos.StagedPolicyEntry, 0, len(current)+len(payloads))
	entries = append(entries, current...)

	for _, payload := range payloads {
		entry, err := s.buildEntry(payload)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
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
