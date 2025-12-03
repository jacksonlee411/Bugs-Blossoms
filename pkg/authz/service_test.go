package authz

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	root := filepath.Join("testdata")
	svc, err := NewService(Config{
		ModelPath:    filepath.Join(root, "model.conf"),
		PolicyPath:   filepath.Join(root, "policy.csv"),
		FlagProvider: staticFlagProvider{mode: ModeEnforce},
	})
	require.NoError(t, err)
	return svc
}

func TestServiceAuthorize(t *testing.T) {
	svc := newTestService(t)
	req := NewRequest(
		SubjectForUser(uuid.Nil, uuid.MustParse("f6f8b13e-755f-41e0-af1a-f2671e40c15c")),
		DomainFromTenant(uuid.Nil),
		ObjectName("core", "users"),
		NormalizeAction("list"),
	)
	require.NoError(t, svc.Authorize(context.Background(), req))
}

func TestServiceAuthorizeDenied(t *testing.T) {
	svc := newTestService(t)
	req := NewRequest(
		SubjectForUser(uuid.Nil, uuid.New()),
		DomainFromTenant(uuid.Nil),
		ObjectName("core", "users"),
		NormalizeAction("edit"),
	)
	err := svc.Authorize(context.Background(), req)
	require.Error(t, err)
}

func TestServiceAuthorizeShadowMode(t *testing.T) {
	root := filepath.Join("testdata")
	svc, err := NewService(Config{
		ModelPath:    filepath.Join(root, "model.conf"),
		PolicyPath:   filepath.Join(root, "policy.csv"),
		FlagProvider: staticFlagProvider{mode: ModeShadow},
	})
	require.NoError(t, err)

	req := NewRequest(
		SubjectForUser(uuid.Nil, uuid.New()),
		DomainFromTenant(uuid.Nil),
		ObjectName("core", "users"),
		NormalizeAction("edit"),
	)
	require.NoError(t, svc.Authorize(context.Background(), req))
}

func TestServiceMode(t *testing.T) {
	root := filepath.Join("testdata")
	svc, err := NewService(Config{
		ModelPath:    filepath.Join(root, "model.conf"),
		PolicyPath:   filepath.Join(root, "policy.csv"),
		FlagProvider: staticFlagProvider{mode: ModeDisabled},
	})
	require.NoError(t, err)
	require.Equal(t, ModeDisabled, svc.Mode())
}
