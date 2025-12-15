package routing

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type RouteClass string

const (
	RouteClassUI          RouteClass = "ui"
	RouteClassAuthn       RouteClass = "authn"
	RouteClassInternalAPI RouteClass = "internal_api"
	RouteClassPublicAPI   RouteClass = "public_api"
	RouteClassWebhook     RouteClass = "webhook"
	RouteClassOps         RouteClass = "ops"
	RouteClassTest        RouteClass = "test"
	RouteClassStatic      RouteClass = "static"
	RouteClassWebsocket   RouteClass = "websocket"
	RouteClassDevOnly     RouteClass = "dev_only"
)

var ErrAllowlistNotFound = errors.New("routing allowlist not found")

type AllowlistRule struct {
	Prefix string     `yaml:"prefix"`
	Class  RouteClass `yaml:"class"`
}

type allowlistFile struct {
	Version     int                        `yaml:"version"`
	Entrypoints map[string][]AllowlistRule `yaml:"entrypoints"`
}

func DefaultAllowlistPath() string {
	if p := strings.TrimSpace(os.Getenv("ROUTING_ALLOWLIST_PATH")); p != "" {
		return p
	}

	const relative = "config/routing/allowlist.yaml"
	if wd, err := os.Getwd(); err == nil {
		if repoRoot, ok := findGoModRoot(wd); ok {
			abs := filepath.Join(repoRoot, filepath.FromSlash(relative))
			if _, statErr := os.Stat(abs); statErr == nil {
				return abs
			}
		}
	}

	return filepath.FromSlash(relative)
}

func LoadAllowlist(path, entrypoint string) ([]AllowlistRule, error) {
	if strings.TrimSpace(path) == "" {
		path = DefaultAllowlistPath()
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrAllowlistNotFound, path)
		}
		return nil, err
	}

	var file allowlistFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		return nil, err
	}

	if file.Version != 1 {
		return nil, fmt.Errorf("unsupported allowlist version: %d", file.Version)
	}

	if strings.TrimSpace(entrypoint) == "" {
		entrypoint = "server"
	}
	rules, ok := file.Entrypoints[entrypoint]
	if !ok {
		return nil, fmt.Errorf("entrypoint %q not found in allowlist", entrypoint)
	}

	for i := range rules {
		rules[i].Prefix = strings.TrimSpace(rules[i].Prefix)
		if rules[i].Prefix == "" {
			return nil, fmt.Errorf("allowlist rule[%d]: empty prefix", i)
		}
		if !strings.HasPrefix(rules[i].Prefix, "/") {
			return nil, fmt.Errorf("allowlist rule[%d]: prefix must start with '/': %q", i, rules[i].Prefix)
		}
		switch rules[i].Class {
		case RouteClassUI,
			RouteClassAuthn,
			RouteClassInternalAPI,
			RouteClassPublicAPI,
			RouteClassWebhook,
			RouteClassOps,
			RouteClassTest,
			RouteClassStatic,
			RouteClassWebsocket,
			RouteClassDevOnly:
		default:
			return nil, fmt.Errorf("allowlist rule[%d]: unknown class: %q", i, rules[i].Class)
		}
	}

	return rules, nil
}

func findGoModRoot(start string) (string, bool) {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}
