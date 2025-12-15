package configuration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnv_FallsBackToGoModRoot(t *testing.T) {
	tmp := t.TempDir()

	requireWriteFile(t, filepath.Join(tmp, "go.mod"), "module example.com/test\n\ngo 1.22\n")
	requireWriteFile(t, filepath.Join(tmp, ".env.local"), "IOTA_SDK_TEST_ENV_LOAD=ok\n")

	sub := filepath.Join(tmp, "pkg", "crud")
	requireMkdirAll(t, sub)

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	if err := os.Chdir(sub); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	_ = os.Unsetenv("IOTA_SDK_TEST_ENV_LOAD")

	n, err := LoadEnv([]string{".env", ".env.local"})
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 env file loaded, got %d", n)
	}
	if got := os.Getenv("IOTA_SDK_TEST_ENV_LOAD"); got != "ok" {
		t.Fatalf("expected env var loaded from repo root, got %q", got)
	}
}

func requireWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func requireMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
