package authz

import (
	"context"
	"fmt"
	"sync"

	"github.com/casbin/casbin/v2"
	fileadapter "github.com/casbin/casbin/v2/persist/file-adapter"
	"github.com/sirupsen/logrus"
)

// Service provides helpers for enforcing authorization decisions.
type Service struct {
	cfg          Config
	enforcer     *casbin.Enforcer
	logger       *logrus.Entry
	flagProvider FlagProvider
	mu           sync.RWMutex
}

// NewService constructs a Service with the provided config.
func NewService(cfg Config) (*Service, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	cfg = cfg.normalized()

	var logger *logrus.Entry
	if cfg.Logger != nil {
		logger = cfg.Logger.WithField("component", "authz")
	} else {
		logger = logrus.WithField("component", "authz")
	}

	enf, err := casbin.NewEnforcer(cfg.ModelPath, fileadapter.NewAdapter(cfg.PolicyPath))
	if err != nil {
		return nil, fmt.Errorf("authz: failed to initialize enforcer: %w", err)
	}
	if err := enf.LoadPolicy(); err != nil {
		return nil, fmt.Errorf("authz: failed to load policies: %w", err)
	}

	provider := cfg.FlagProvider
	if provider == nil {
		provider = NewFileFlagProvider(cfg.FlagPath, cfg.FlagMode)
	}

	return &Service{
		cfg:          cfg,
		enforcer:     enf,
		logger:       logger,
		flagProvider: provider,
	}, nil
}

// Authorize returns an error if the request is denied.
func (s *Service) Authorize(ctx context.Context, req Request) error {
	switch mode := s.flagProvider.Mode(); mode {
	case ModeDisabled:
		return nil
	case ModeShadow:
		allowed, err := s.Check(ctx, req)
		if err != nil {
			return err
		}
		if !allowed {
			s.logger.WithContext(ctx).WithFields(logrus.Fields{
				"subject": req.Subject,
				"domain":  req.Domain,
				"object":  req.Object,
				"action":  req.Action,
				"mode":    ModeShadow,
			}).Warn("authz shadow deny")
		}
		return nil
	case ModeEnforce:
		allowed, err := s.Check(ctx, req)
		if err != nil {
			return err
		}
		if !allowed {
			s.logger.WithContext(ctx).WithFields(logrus.Fields{
				"subject": req.Subject,
				"domain":  req.Domain,
				"object":  req.Object,
				"action":  req.Action,
				"mode":    ModeEnforce,
			}).Warn("authz denied request")
			return forbiddenError(req)
		}
		return nil
	default:
		s.logger.WithContext(ctx).WithField("mode", mode).Warn("authz: unknown flag mode, defaulting to shadow")
		allowed, err := s.Check(ctx, req)
		if err != nil {
			return err
		}
		if !allowed {
			s.logger.WithContext(ctx).WithFields(logrus.Fields{
				"subject": req.Subject,
				"domain":  req.Domain,
				"object":  req.Object,
				"action":  req.Action,
				"mode":    ModeShadow,
			}).Warn("authz shadow deny")
		}
		return nil
	}
}

// Check evaluates a request without returning an authorization error.
func (s *Service) Check(ctx context.Context, req Request) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res, err := s.enforcer.Enforce(req.Subject, req.Domain, req.Object, req.Action, req.Attributes)
	if err != nil {
		return false, fmt.Errorf("authz: enforce failed: %w", err)
	}
	return res, nil
}

// ReloadPolicy reloads policy data from disk.
func (s *Service) ReloadPolicy(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.enforcer.LoadPolicy(); err != nil {
		return fmt.Errorf("authz: reload policy failed: %w", err)
	}
	s.logger.WithContext(ctx).Info("authz policy reloaded")
	return nil
}

// Enforcer exposes the underlying casbin enforcer (read-only usage only).
func (s *Service) Enforcer() *casbin.Enforcer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enforcer
}

var (
	defaultServiceOnce sync.Once
	defaultService     *Service
	defaultServiceErr  error
)

// Use returns a singleton Service configured via environment variables.
func Use() *Service {
	defaultServiceOnce.Do(func() {
		defaultService, defaultServiceErr = NewService(DefaultConfig())
	})
	if defaultServiceErr != nil {
		panic(defaultServiceErr)
	}
	return defaultService
}
