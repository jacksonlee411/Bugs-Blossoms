package authz

import (
	"os"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Mode represents the global enforcement mode.
type Mode string

const (
	ModeDisabled Mode = "disabled"
	ModeShadow   Mode = "shadow"
	ModeEnforce  Mode = "enforce"
)

// FlagProvider supplies the current enforcement mode.
type FlagProvider interface {
	Mode() Mode
}

type staticFlagProvider struct {
	mode Mode
}

func (s staticFlagProvider) Mode() Mode {
	return s.mode
}

// FileFlagProvider loads authz flags from a YAML file.
type FileFlagProvider struct {
	path     string
	fallback Mode
	lastMode Mode
	mu       sync.Mutex
}

// NewFileFlagProvider returns a provider backed by a YAML config file.
func NewFileFlagProvider(path string, fallback Mode) FlagProvider {
	return &FileFlagProvider{
		path:     path,
		fallback: sanitizeMode(fallback),
	}
}

func (p *FileFlagProvider) Mode() Mode {
	p.mu.Lock()
	defer p.mu.Unlock()

	data, err := os.ReadFile(p.path)
	if err != nil {
		if p.lastMode == "" {
			p.lastMode = p.fallback
		}
		return p.lastMode
	}

	var cfg struct {
		Mode string `yaml:"mode"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return p.fallback
	}
	p.lastMode = sanitizeMode(Mode(cfg.Mode))
	return p.lastMode
}

func sanitizeMode(mode Mode) Mode {
	switch strings.ToLower(string(mode)) {
	case string(ModeDisabled):
		return ModeDisabled
	case string(ModeEnforce):
		return ModeEnforce
	default:
		return ModeShadow
	}
}
