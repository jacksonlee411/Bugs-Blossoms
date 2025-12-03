package main

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func discoverRepoRoot() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("AUTHZ_BOT_REPO_DIR")); dir != "" {
		return filepath.Abs(dir)
	}
	if out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	return os.Getwd()
}

func absPath(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func defaultLockerID() string {
	host, _ := os.Hostname()
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

func resolveRepoSlug(root, remote string) (string, string, error) {
	if slug := strings.TrimSpace(os.Getenv("AUTHZ_BOT_REPO_SLUG")); slug != "" {
		return splitSlug(slug)
	}
	cmd := exec.Command("git", "-C", root, "config", fmt.Sprintf("remote.%s.url", remote))
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to read remote %s url: %w", remote, err)
	}
	return parseRemoteURL(strings.TrimSpace(string(out)))
}

func splitSlug(slug string) (string, string, error) {
	parts := strings.Split(strings.Trim(slug, "/"), "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid repo slug %q", slug)
	}
	return parts[0], parts[1], nil
}

func parseRemoteURL(raw string) (string, string, error) {
	trimmed := strings.TrimSuffix(raw, ".git")
	switch {
	case strings.HasPrefix(trimmed, "git@"):
		idx := strings.Index(trimmed, ":")
		if idx == -1 {
			return "", "", fmt.Errorf("invalid ssh remote: %s", raw)
		}
		return splitSlug(trimmed[idx+1:])
	case strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://"):
		u, err := url.Parse(trimmed)
		if err != nil {
			return "", "", err
		}
		return splitSlug(strings.TrimPrefix(u.Path, "/"))
	default:
		return splitSlug(trimmed)
	}
}

func defaultBaseBranch(root string) string {
	if branch := strings.TrimSpace(os.Getenv("AUTHZ_BOT_BASE_BRANCH")); branch != "" {
		return branch
	}
	if out, err := exec.Command("git", "-C", root, "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return "main"
}
