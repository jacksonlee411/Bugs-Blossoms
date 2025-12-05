package testhelpers

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/stretchr/testify/require"
)

var authzFlagMu sync.Mutex

// WithAuthzMode rewrites config/access/authz_flags.yaml for the duration of a test.
func WithAuthzMode(t *testing.T, mode authz.Mode) {
	t.Helper()

	authzFlagMu.Lock()

	flagPath := os.Getenv("AUTHZ_FLAG_CONFIG")
	if flagPath == "" {
		flagPath = filepath.Join("config", "access", "authz_flags.yaml")
	}

	tmpDir := t.TempDir()
	tmpFlagPath := filepath.Join(tmpDir, "authz_flags.yaml")
	newContent := []byte(fmt.Sprintf("mode: %s\n", mode))
	require.NoError(t, os.WriteFile(tmpFlagPath, newContent, 0o644))
	t.Setenv("AUTHZ_FLAG_CONFIG", tmpFlagPath)
	authz.Reset()

	t.Cleanup(func() {
		authz.Reset()
		authzFlagMu.Unlock()
	})
}
