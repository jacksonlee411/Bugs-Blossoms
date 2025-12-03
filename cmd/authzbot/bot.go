package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"

	authzPersistence "github.com/iota-uz/iota-sdk/pkg/authz/persistence"
	authzVersion "github.com/iota-uz/iota-sdk/pkg/authz/version"
	"github.com/iota-uz/iota-sdk/pkg/composables"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BotConfig defines runtime options for the authz bot.
type BotConfig struct {
	LockerID       string
	PollInterval   time.Duration
	RootDir        string
	PolicyDir      string
	PolicyPath     string
	RevisionPath   string
	BranchPrefix   string
	RemoteName     string
	BaseBranch     string
	RepoOwner      string
	RepoName       string
	GitAuthorName  string
	GitAuthorEmail string
	GitHubToken    string
	GitHubAPI      string
	RunOnce        bool
}

// Bot processes approved policy drafts and turns them into Git changes.
type Bot struct {
	cfg             BotConfig
	repo            authzPersistence.PolicyChangeRequestRepository
	pool            *pgxpool.Pool
	versionProvider authzVersion.Provider
	logger          *logrus.Entry
	git             *gitRunner
}

var (
	defaultGitHubAPI  = "https://api.github.com"
	errNoPendingDraft = errors.New("authzbot: no pending drafts")
)

// NewBot wires the dependencies together.
func NewBot(cfg BotConfig, repo authzPersistence.PolicyChangeRequestRepository, pool *pgxpool.Pool, provider authzVersion.Provider, baseLogger *logrus.Logger) *Bot {
	api := cfg.GitHubAPI
	if strings.TrimSpace(api) == "" {
		api = defaultGitHubAPI
	}
	logger := baseLogger.WithField("component", "authzbot")
	return &Bot{
		cfg:             cfg,
		repo:            repo,
		pool:            pool,
		versionProvider: provider,
		logger:          logger,
		git:             newGitRunner(cfg, api, logger),
	}
}

// Run starts the polling loop.
func (b *Bot) Run(ctx context.Context) error {
	ticker := time.NewTicker(b.cfg.PollInterval)
	defer ticker.Stop()

	for {
		if err := b.iteration(ctx); err != nil {
			b.logger.WithError(err).Error("bot iteration failed")
		}
		if b.cfg.RunOnce {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (b *Bot) iteration(ctx context.Context) error {
	ctx = composables.WithPool(ctx, b.pool)
	req, err := b.nextCandidate(ctx)
	if err != nil {
		if errors.Is(err, errNoPendingDraft) {
			b.logger.Debug("no pending drafts found")
			return nil
		}
		return err
	}

	if err := b.process(ctx, req); err != nil {
		b.logger.WithError(err).WithField("request_id", req.ID).Error("failed to process draft")
		return err
	}
	return nil
}

func (b *Bot) nextCandidate(ctx context.Context) (*authzPersistence.PolicyChangeRequest, error) {
	params := authzPersistence.FindParams{
		Statuses: []authzPersistence.PolicyChangeStatus{
			authzPersistence.PolicyChangeStatusApproved,
			authzPersistence.PolicyChangeStatusFailed,
		},
		Limit:   50,
		SortAsc: true,
	}
	list, _, err := b.repo.List(ctx, params)
	if err != nil {
		return nil, err
	}
	for i := range list {
		req := list[i]
		if req.Status == authzPersistence.PolicyChangeStatusFailed {
			if req.ErrorLog != nil && strings.TrimSpace(*req.ErrorLog) != "" {
				continue
			}
		}
		return &req, nil
	}
	return nil, errNoPendingDraft
}

func (b *Bot) process(ctx context.Context, req *authzPersistence.PolicyChangeRequest) error {
	now := time.Now().UTC()
	stale := now.Add(-5 * time.Minute)
	locked, err := b.repo.AcquireBotLock(ctx, req.ID, authzPersistence.BotLockParams{
		Locker:      b.cfg.LockerID,
		LockedAt:    now,
		StaleBefore: stale,
	})
	if err != nil {
		return err
	}
	if !locked {
		b.logger.WithField("request_id", req.ID).Debug("unable to acquire lock")
		return nil
	}
	defer b.repo.ReleaseBotLock(ctx, req.ID, b.cfg.LockerID) //nolint:errcheck

	if err := b.repo.UpdateBotMetadata(ctx, req.ID, authzPersistence.UpdateBotMetadataParams{
		BotJobID:    authzPersistence.NewNullableValue(fmt.Sprintf("%s-%d", b.cfg.LockerID, now.Unix())),
		BotAttempts: authzPersistence.NewNullableValue(req.BotAttempts + 1),
		ErrorLog:    authzPersistence.NewNullableNull[string](),
	}); err != nil {
		return err
	}

	return b.applyAndCommit(ctx, req)
}

func (b *Bot) applyAndCommit(ctx context.Context, req *authzPersistence.PolicyChangeRequest) error {
	branchName := fmt.Sprintf("%s%s", b.cfg.BranchPrefix, req.ID.String())
	cleanup, err := b.git.prepareBranch(ctx, branchName)
	if err != nil {
		b.failRequest(ctx, req.ID, err)
		return err
	}
	defer cleanup()

	meta, err := b.versionProvider.Current(ctx)
	if err != nil {
		b.failRequest(ctx, req.ID, err)
		return err
	}
	if req.BasePolicyRevision != "" && req.BasePolicyRevision != meta.Revision {
		err := fmt.Errorf("base revision mismatch: have %s, request %s", meta.Revision, req.BasePolicyRevision)
		b.failRequest(ctx, req.ID, err)
		return err
	}

	store, err := loadPolicyStore(b.cfg.PolicyDir)
	if err != nil {
		b.failRequest(ctx, req.ID, err)
		return err
	}
	reversePatch, err := store.Apply(req.Diff)
	if err != nil {
		b.failRequest(ctx, req.ID, err)
		return err
	}

	if err := runMake(ctx, b.cfg.RootDir, "authz-pack"); err != nil {
		b.failRequest(ctx, req.ID, err)
		return err
	}
	if err := runMake(ctx, b.cfg.RootDir, "authz-test"); err != nil {
		b.failRequest(ctx, req.ID, err)
		return err
	}

	if err := b.git.add(ctx,
		b.cfg.PolicyDir,
		b.cfg.PolicyPath,
		b.cfg.RevisionPath,
	); err != nil {
		b.failRequest(ctx, req.ID, err)
		return err
	}
	commitMsg := fmt.Sprintf("chore(authz): apply policy request %s", req.ID)
	if err := b.git.commit(ctx, commitMsg); err != nil {
		b.failRequest(ctx, req.ID, err)
		return err
	}
	if err := b.git.push(ctx, branchName); err != nil {
		b.failRequest(ctx, req.ID, err)
		return err
	}

	body := renderPRBody(req)
	prURL, err := b.git.createPR(ctx, branchName, fmt.Sprintf("Authz policy update %s", req.ID), body)
	if err != nil {
		b.failRequest(ctx, req.ID, err)
		return err
	}

	newMeta, err := b.versionProvider.Current(ctx)
	if err != nil {
		b.failRequest(ctx, req.ID, err)
		return err
	}

	if err := b.repo.UpdateBotMetadata(ctx, req.ID, authzPersistence.UpdateBotMetadataParams{
		PRLink:                authzPersistence.NewNullableValue(prURL),
		AppliedPolicyRevision: authzPersistence.NewNullableValue(newMeta.Revision),
		AppliedPolicySnapshot: authzPersistence.NewNullableValue(reversePatch),
	}); err != nil {
		b.failRequest(ctx, req.ID, err)
		return err
	}
	if err := b.repo.UpdateStatus(ctx, req.ID, authzPersistence.UpdateStatusParams{Status: authzPersistence.PolicyChangeStatusMerged}); err != nil {
		b.failRequest(ctx, req.ID, err)
		return err
	}

	b.logger.WithField("request_id", req.ID).WithField("pr", prURL).Info("policy draft submitted")
	return nil
}

func (b *Bot) failRequest(ctx context.Context, id uuid.UUID, failure error) {
	msg := failure.Error()
	_ = b.repo.UpdateBotMetadata(ctx, id, authzPersistence.UpdateBotMetadataParams{
		ErrorLog: authzPersistence.NewNullableValue(msg),
	})
	_ = b.repo.UpdateStatus(ctx, id, authzPersistence.UpdateStatusParams{Status: authzPersistence.PolicyChangeStatusFailed})
}

func renderPRBody(req *authzPersistence.PolicyChangeRequest) string {
	return fmt.Sprintf(`## Policy Draft %s

- Subject: %s
- Domain: %s
- Object: %s
- Action: %s
- Reason: %s
- Base Revision: %s
`,
		req.ID,
		req.Subject,
		req.Domain,
		req.Object,
		req.Action,
		sanitizeReason(req.Reason),
		req.BasePolicyRevision,
	)
}

func sanitizeReason(reason string) string {
	if strings.TrimSpace(reason) == "" {
		return "(not provided)"
	}
	return reason
}
