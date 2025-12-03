package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sirupsen/logrus"

	authzPersistence "github.com/iota-uz/iota-sdk/pkg/authz/persistence"
	authzVersion "github.com/iota-uz/iota-sdk/pkg/authz/version"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

func main() {
	var (
		forceRelease = flag.String("force-release", "", "release bot lock for the given request ID and exit")
		runOnce      = flag.Bool("once", false, "process at most one request and exit")
	)
	flag.Parse()

    conf := configuration.Use()
    baseLogger := conf.Logger()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

    pool, err := pgxpool.New(ctx, conf.Database.Opts)
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}
	defer pool.Close()

	repo := authzPersistence.NewPolicyChangeRequestRepository()

    if id := strings.TrimSpace(*forceRelease); id != "" {
        handleForceRelease(ctx, baseLogger, pool, repo, id)
        return
    }

    cfg, err := buildBotConfig()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}
	cfg.RunOnce = *runOnce

	provider := authzVersion.NewFileProvider(cfg.RevisionPath)
    bot := NewBot(cfg, repo, pool, provider, baseLogger)

	if err := bot.Run(composables.WithPool(ctx, pool)); err != nil {
		log.Fatalf("bot terminated: %v", err)
	}
}

func handleForceRelease(ctx context.Context, logger *logrus.Logger, pool *pgxpool.Pool, repo authzPersistence.PolicyChangeRequestRepository, rawID string) {
	requestID, err := uuid.Parse(rawID)
	if err != nil {
		log.Fatalf("invalid request ID: %v", err)
	}
	ctx = composables.WithPool(ctx, pool)
	if err := repo.ForceReleaseBotLock(ctx, requestID); err != nil {
		log.Fatalf("failed to release lock: %v", err)
	}
	logger.WithField("request_id", requestID).Info("released bot lock")
}

func buildBotConfig() (BotConfig, error) {
	root, err := discoverRepoRoot()
	if err != nil {
		return BotConfig{}, err
	}
	policyDir := envOrDefault("AUTHZ_BOT_POLICY_DIR", "config/access/policies")
	policyPath := envOrDefault("AUTHZ_BOT_POLICY_FILE", "config/access/policy.csv")
	revisionPath := envOrDefault("AUTHZ_BOT_REVISION_FILE", policyPath+".rev")

	pollInterval := envDuration("AUTHZ_BOT_POLL_INTERVAL", 30*time.Second)
	locker := envOrDefault("AUTHZ_BOT_LOCKER", defaultLockerID())
	branchPrefix := envOrDefault("AUTHZ_BOT_GIT_BRANCH_PREFIX", "authz/bot/")
	remoteName := envOrDefault("AUTHZ_BOT_GIT_REMOTE", "origin")
	baseBranch := defaultBaseBranch(root)
	authorName := envOrDefault("AUTHZ_BOT_GIT_AUTHOR", "Authz Bot")
	authorEmail := envOrDefault("AUTHZ_BOT_GIT_EMAIL", "authz-bot@example.com")
	repoOwner, repoName, err := resolveRepoSlug(root, remoteName)
	if err != nil {
		return BotConfig{}, err
	}
	token := strings.TrimSpace(os.Getenv("AUTHZ_BOT_GITHUB_TOKEN"))
	if token == "" {
		return BotConfig{}, fmt.Errorf("AUTHZ_BOT_GITHUB_TOKEN is required")
	}

	cfg := BotConfig{
		LockerID:       locker,
		PollInterval:   pollInterval,
		RootDir:        root,
		PolicyDir:      absPath(root, policyDir),
		PolicyPath:     absPath(root, policyPath),
		RevisionPath:   absPath(root, revisionPath),
		BranchPrefix:   branchPrefix,
		RemoteName:     remoteName,
		BaseBranch:     baseBranch,
		RepoOwner:      repoOwner,
		RepoName:       repoName,
		GitAuthorName:  authorName,
		GitAuthorEmail: authorEmail,
		GitHubToken:    token,
	}
	if api := strings.TrimSpace(os.Getenv("AUTHZ_BOT_GITHUB_API")); api != "" {
		cfg.GitHubAPI = api
	}
	return cfg, nil
}

func envOrDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
