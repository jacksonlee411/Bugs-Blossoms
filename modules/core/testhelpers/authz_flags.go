package testhelpers

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/stretchr/testify/require"
)

var authzFlagMu sync.Mutex

// WithAuthzMode rewrites config/access/authz_flags.yaml for the duration of a test.
func WithAuthzMode(t *testing.T, mode authz.Mode) {
	t.Helper()

	authzFlagMu.Lock()

	tmpDir := t.TempDir()
	tmpFlagPath := filepath.Join(tmpDir, "authz_flags.yaml")
	newContent := []byte(fmt.Sprintf("mode: %s\n", mode))
	require.NoError(t, os.WriteFile(tmpFlagPath, newContent, 0o644))
	t.Setenv("AUTHZ_FLAG_CONFIG", tmpFlagPath)

	cfg := configuration.Use()
	origFlagPath := cfg.Authz.FlagConfigPath
	origMode := cfg.Authz.Mode
	cfg.Authz.FlagConfigPath = tmpFlagPath
	cfg.Authz.Mode = string(mode)

	authz.Reset()

	t.Cleanup(func() {
		cfg.Authz.FlagConfigPath = origFlagPath
		cfg.Authz.Mode = origMode
		authz.Reset()
		authzFlagMu.Unlock()
	})
}
