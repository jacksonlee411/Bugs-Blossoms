package version

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// Metadata captures revision metadata for aggregated policy files.
type Metadata struct {
	Revision    string    `json:"revision"`
	GeneratedAt time.Time `json:"generated_at"`
	Entries     int       `json:"entries"`
}

// Provider exposes the current policy revision metadata.
type Provider interface {
	Current(ctx context.Context) (Metadata, error)
}

// FileProvider reads revision metadata from a JSON file produced by authz-pack.
type FileProvider struct {
	path       string
	mu         sync.RWMutex
	cachedMeta Metadata
	cachedMod  time.Time
}

// NewFileProvider constructs a Provider backed by the given revision file path.
func NewFileProvider(path string) *FileProvider {
	return &FileProvider{path: path}
}

// Current returns the latest metadata, caching file contents between calls.
func (p *FileProvider) Current(_ context.Context) (Metadata, error) {
	if p.path == "" {
		return Metadata{}, errors.New("authz/version: revision path is not configured")
	}

	info, err := os.Stat(p.path)
	if err != nil {
		return Metadata{}, fmt.Errorf("authz/version: stat revision file: %w", err)
	}

	p.mu.RLock()
	if !p.cachedMod.IsZero() && !info.ModTime().After(p.cachedMod) && p.cachedMeta.Revision != "" {
		meta := p.cachedMeta
		p.mu.RUnlock()
		return meta, nil
	}
	p.mu.RUnlock()

	data, err := os.ReadFile(p.path)
	if err != nil {
		return Metadata{}, fmt.Errorf("authz/version: read revision file: %w", err)
	}

	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return Metadata{}, fmt.Errorf("authz/version: parse revision file: %w", err)
	}
	if meta.Revision == "" {
		return Metadata{}, errors.New("authz/version: revision field is empty")
	}

	p.mu.Lock()
	p.cachedMeta = meta
	p.cachedMod = info.ModTime()
	p.mu.Unlock()

	return meta, nil
}
