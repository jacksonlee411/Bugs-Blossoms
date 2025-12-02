package authz

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

// Config captures all inputs necessary to initialize the Casbin enforcer.
type Config struct {
	ModelPath    string
	PolicyPath   string
	FlagPath     string
	FlagMode     Mode
	Logger       *logrus.Logger
	FlagProvider FlagProvider
}

func (c Config) validate() error {
	if c.ModelPath == "" {
		return configError("missing model path")
	}
	if c.PolicyPath == "" {
		return configError("missing policy path")
	}
	if c.FlagPath == "" && c.FlagProvider == nil {
		return configError("missing flag configuration path")
	}
	return nil
}

func (c Config) normalized() Config {
	c.ModelPath = filepath.Clean(c.ModelPath)
	c.PolicyPath = filepath.Clean(c.PolicyPath)
	if c.FlagPath != "" {
		c.FlagPath = filepath.Clean(c.FlagPath)
	}
	return c
}

// DefaultConfig builds a Config using the global configuration singleton.
func DefaultConfig() Config {
	cfg := configuration.Use()
	mode := sanitizeMode(Mode(cfg.Authz.Mode))
	if envMode := strings.TrimSpace(os.Getenv("AUTHZ_MODE")); envMode != "" {
		mode = sanitizeMode(Mode(envMode))
	}

	return Config{
		ModelPath:  cfg.Authz.ModelPath,
		PolicyPath: cfg.Authz.PolicyPath,
		FlagPath:   cfg.Authz.FlagConfigPath,
		FlagMode:   mode,
		Logger:     cfg.Logger(),
	}
}
