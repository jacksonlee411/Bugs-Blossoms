package persistence

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/modules/website/domain/entities/chatthread"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

type SafeMap[K comparable, V any] struct {
	mu sync.RWMutex
	m  map[K]V
}

func NewSafeMap[K comparable, V any]() *SafeMap[K, V] {
	return &SafeMap[K, V]{
		m: make(map[K]V),
	}
}

func (s *SafeMap[K, V]) Set(key K, value V) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = value
}

func (s *SafeMap[K, V]) Get(key K) (V, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, found := s.m[key]
	return val, found
}

func (s *SafeMap[K, V]) Delete(key K) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
}

func (s *SafeMap[K, V]) Values() []V {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Collect(maps.Values(s.m))
}

type threadKey struct {
	tenantID uuid.UUID
	threadID uuid.UUID
}

type InmemThreadRepository struct {
	storage *SafeMap[threadKey, chatthread.ChatThread]
}

func NewInmemThreadRepository() *InmemThreadRepository {
	return &InmemThreadRepository{
		storage: NewSafeMap[threadKey, chatthread.ChatThread](),
	}
}

func (r *InmemThreadRepository) GetByID(ctx context.Context, id uuid.UUID) (chatthread.ChatThread, error) {
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return nil, err
	}
	thread, found := r.storage.Get(threadKey{tenantID: tenantID, threadID: id})
	if !found {
		return nil, chatthread.ErrChatThreadNotFound
	}
	return thread, nil
}

func (r *InmemThreadRepository) Save(ctx context.Context, thread chatthread.ChatThread) (chatthread.ChatThread, error) {
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return nil, err
	}
	if thread.TenantID() != tenantID {
		return nil, errors.New("thread tenant mismatch")
	}
	r.storage.Set(threadKey{tenantID: tenantID, threadID: thread.ID()}, thread)
	return thread, nil
}

func (r *InmemThreadRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return err
	}
	r.storage.Delete(threadKey{tenantID: tenantID, threadID: id})
	return nil
}

func (r *InmemThreadRepository) List(ctx context.Context) ([]chatthread.ChatThread, error) {
	tenantID, err := composables.UseTenantID(ctx)
	if err != nil {
		return nil, err
	}
	allThreads := r.storage.Values()
	threads := make([]chatthread.ChatThread, 0, len(allThreads))
	for _, thread := range allThreads {
		if thread.TenantID() == tenantID {
			threads = append(threads, thread)
		}
	}
	return threads, nil
}
