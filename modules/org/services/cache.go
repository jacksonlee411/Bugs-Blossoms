package services

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

type orgCache struct {
	mu          sync.RWMutex
	entries     map[string]any
	tenantIndex map[uuid.UUID]map[string]struct{}
}

func newOrgCache() *orgCache {
	return &orgCache{
		entries:     make(map[string]any),
		tenantIndex: make(map[uuid.UUID]map[string]struct{}),
	}
}

func (c *orgCache) Get(key string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.entries[key]
	return v, ok
}

func (c *orgCache) Set(tenantID uuid.UUID, key string, value any) {
	if tenantID == uuid.Nil || key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = value
	if _, ok := c.tenantIndex[tenantID]; !ok {
		c.tenantIndex[tenantID] = make(map[string]struct{})
	}
	c.tenantIndex[tenantID][key] = struct{}{}
}

func (c *orgCache) InvalidateTenant(tenantID uuid.UUID) {
	if tenantID == uuid.Nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := c.tenantIndex[tenantID]
	for key := range keys {
		delete(c.entries, key)
	}
	delete(c.tenantIndex, tenantID)
}

type cachedHierarchy struct {
	Nodes []HierarchyNode
	AsOf  time.Time
}

type cachedAssignments struct {
	SubjectID uuid.UUID
	Rows      []AssignmentViewRow
	AsOf      time.Time
}
