package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

type gitRunner struct {
	root        string
	remote      string
	baseBranch  string
	authorName  string
	authorEmail string
	repoOwner   string
	repoName    string
	token       string
	apiURL      string
	logger      *logrus.Entry
	httpClient  *http.Client
}

func newGitRunner(cfg BotConfig, api string, logger *logrus.Entry) *gitRunner {
	return &gitRunner{
		root:        cfg.RootDir,
		remote:      cfg.RemoteName,
		baseBranch:  cfg.BaseBranch,
		authorName:  cfg.GitAuthorName,
		authorEmail: cfg.GitAuthorEmail,
		repoOwner:   cfg.RepoOwner,
		repoName:    cfg.RepoName,
		token:       cfg.GitHubToken,
		apiURL:      strings.TrimRight(api, "/"),
		logger:      logger,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (g *gitRunner) prepareBranch(ctx context.Context, branch string) (func(), error) {
	if err := g.ensureClean(ctx); err != nil {
		return nil, err
	}
	if err := g.run(ctx, "fetch", g.remote, g.baseBranch); err != nil {
		return nil, err
	}
	if err := g.run(ctx, "checkout", g.baseBranch); err != nil {
		return nil, err
	}
	if err := g.run(ctx, "reset", "--hard", fmt.Sprintf("%s/%s", g.remote, g.baseBranch)); err != nil {
		return nil, err
	}
	_ = g.run(ctx, "branch", "-D", branch)
	if err := g.run(ctx, "checkout", "-b", branch); err != nil {
		return nil, err
	}
	if err := g.configureUser(ctx); err != nil {
		return nil, err
	}
	cleanup := func() {
		_ = g.run(ctx, "checkout", g.baseBranch)
		_ = g.run(ctx, "branch", "-D", branch)
	}
	return cleanup, nil
}

func (g *gitRunner) ensureClean(ctx context.Context) error {
	out, err := g.command(ctx, "status", "--porcelain").Output()
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(out)) != "" {
		return fmt.Errorf("working tree is dirty")
	}
	return nil
}

func (g *gitRunner) configureUser(ctx context.Context) error {
	if err := g.run(ctx, "config", "user.name", g.authorName); err != nil {
		return err
	}
	return g.run(ctx, "config", "user.email", g.authorEmail)
}

func (g *gitRunner) add(ctx context.Context, paths ...string) error {
	filtered := make([]string, 0, len(paths))
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		filtered = append(filtered, relativePath(g.root, p))
	}
	if len(filtered) == 0 {
		return nil
	}
	args := append([]string{"add"}, filtered...)
	return g.run(ctx, args...)
}

func (g *gitRunner) commit(ctx context.Context, message string) error {
	return g.run(ctx, "commit", "-m", message)
}

func (g *gitRunner) push(ctx context.Context, branch string) error {
	return g.run(ctx, "push", g.remote, branch)
}

func (g *gitRunner) createPR(ctx context.Context, branch, title, body string) (string, error) {
	payload := map[string]string{
		"title": title,
		"head":  fmt.Sprintf("%s:%s", g.repoOwner, branch),
		"base":  g.baseBranch,
		"body":  body,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	url := fmt.Sprintf("%s/repos/%s/%s/pulls", g.apiURL, g.repoOwner, g.repoName)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	g.decorateRequest(req)
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnprocessableEntity {
		if existing, err := g.findExistingPR(ctx, branch); err == nil && existing != "" {
			return existing, nil
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return "", fmt.Errorf("create PR failed (read body): %w", readErr)
		}
		return "", fmt.Errorf("create PR failed: %s", strings.TrimSpace(string(bodyBytes)))
	}
	var result struct {
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.HTMLURL == "" {
		return "", fmt.Errorf("empty PR url")
	}
	return result.HTMLURL, nil
}

func (g *gitRunner) findExistingPR(ctx context.Context, branch string) (string, error) {
	head := fmt.Sprintf("%s:%s", g.repoOwner, branch)
	endpoint := fmt.Sprintf("%s/repos/%s/%s/pulls?head=%s&state=open", g.apiURL, g.repoOwner, g.repoName, url.QueryEscape(head))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	g.decorateRequest(req)
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("find PR status %s", resp.Status)
	}
	var payload []struct {
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if len(payload) == 0 {
		return "", fmt.Errorf("no existing PR")
	}
	return payload[0].HTMLURL, nil
}

func (g *gitRunner) decorateRequest(req *http.Request) {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", g.token))
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "authzbot")
}

func (g *gitRunner) run(ctx context.Context, args ...string) error {
	cmd := g.command(ctx, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (g *gitRunner) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.root
	return cmd
}

func relativePath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}
