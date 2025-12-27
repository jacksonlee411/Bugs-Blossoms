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
	typ := strings.ToLower(strings.TrimSpace(payload.Type))
	if typ == "" {
		return dtos.StagedPolicyEntry{}, errors.New("type is required")
	}
	if typ == "g2" {
		return dtos.StagedPolicyEntry{}, errors.New("type must be p or g")
	}
	if strings.TrimSpace(payload.Object) == "" {
		return dtos.StagedPolicyEntry{}, errors.New("object is required")
	}
	if strings.TrimSpace(payload.Domain) == "" {
		return dtos.StagedPolicyEntry{}, errors.New("domain is required")
	}

	action := strings.TrimSpace(payload.Action)
	effect := strings.TrimSpace(payload.Effect)
	isGrouping := typ == "g" || typ == "g2"
	if isGrouping {
		if action == "" {
			action = "*"
		}
		effect = "allow"
	} else {
		if action == "" {
			return dtos.StagedPolicyEntry{}, errors.New("action is required")
		}
		if effect == "" {
			effect = "allow"
		}
		if !strings.EqualFold(effect, "allow") {
			return dtos.StagedPolicyEntry{}, errors.New("effect must be allow")
		}
	}

	stageKind, err := normalizeStageKind(payload.StageKind)
	if err != nil {
		return dtos.StagedPolicyEntry{}, err
	}

	return dtos.StagedPolicyEntry{
		ID:        uuid.New().String(),
		StageKind: stageKind,
		PolicyEntryResponse: dtos.PolicyEntryResponse{
			Type:    typ,
			Subject: payload.Subject,
			Domain:  payload.Domain,
			Object:  payload.Object,
			Action:  authz.NormalizeAction(action),
			Effect:  effect,
		},
	}, nil
}

func policyStageKey(userID uint, tenantID uuid.UUID) string {
	return fmt.Sprintf("%d:%s", userID, tenantID.String())
}

func (s *policyStageStore) Add(key string, payload dtos.StagePolicyRequest) ([]dtos.StagedPolicyEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, _, err := s.addAllLocked(key, []dtos.StagePolicyRequest{payload})
	return entries, err
}

func (s *policyStageStore) AddMany(key string, payloads []dtos.StagePolicyRequest) ([]dtos.StagedPolicyEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, _, err := s.addAllLocked(key, payloads)
	return entries, err
}

func (s *policyStageStore) AddManyWithIDs(key string, payloads []dtos.StagePolicyRequest) ([]dtos.StagedPolicyEntry, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.addAllLocked(key, payloads)
}

func (s *policyStageStore) addAllLocked(key string, payloads []dtos.StagePolicyRequest) ([]dtos.StagedPolicyEntry, []string, error) {
	if len(payloads) == 0 {
		return nil, nil, errors.New("at least one staged policy is required")
	}

	current := s.data[key]
	if len(current)+len(payloads) > 50 {
		return nil, nil, errors.New("stage limit reached (50)")
	}

	entries := make([]dtos.StagedPolicyEntry, 0, len(current)+len(payloads))
	entries = append(entries, current...)
	createdIDs := make([]string, 0, len(payloads))

	for _, payload := range payloads {
		entry, err := s.buildEntry(payload)
		if err != nil {
			return nil, nil, err
		}
		createdIDs = append(createdIDs, entry.ID)
		entries = append(entries, entry)
	}
	s.data[key] = entries
	return entries, createdIDs, nil
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

func (s *policyStageStore) DeleteMany(key string, ids []string) ([]dtos.StagedPolicyEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, ok := s.data[key]
	if !ok {
		return []dtos.StagedPolicyEntry{}, nil
	}
	if len(entries) == 0 {
		return []dtos.StagedPolicyEntry{}, nil
	}
	idSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		idSet[id] = struct{}{}
	}
	if len(idSet) == 0 {
		return nil, errors.New("at least one stage entry id is required")
	}

	next := make([]dtos.StagedPolicyEntry, 0, len(entries))
	removed := false
	for _, entry := range entries {
		if _, ok := idSet[entry.ID]; ok {
			removed = true
			continue
		}
		next = append(next, entry)
	}
	if !removed {
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
	subject = strings.TrimSpace(subject)
	domain = strings.TrimSpace(domain)
	if subject == "" && domain == "" {
		delete(s.data, key)
		return 0
	}
	next := make([]dtos.StagedPolicyEntry, 0, len(entries))
	for _, entry := range entries {
		if subject != "" && entry.Subject != subject {
			next = append(next, entry)
			continue
		}
		if domain != "" && entry.Domain != domain {
			next = append(next, entry)
			continue
		}
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
