package services

import (
	"context"
	"crypto/rand"
	stderrors "errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/modules/superadmin/domain"
	"github.com/iota-uz/iota-sdk/modules/superadmin/domain/entities"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pkg/errors"
)

var (
	ErrTenantDomainInvalidHostname = errors.New("invalid hostname")
	ErrTenantDomainHostnameTaken   = errors.New("hostname already bound to another tenant")
	ErrTenantDomainNotFound        = errors.New("domain not found")
	ErrTenantDomainNotVerified     = errors.New("domain not verified")
	ErrTenantDomainIsPrimary       = errors.New("cannot delete primary domain")
)

type TenantDomainService struct {
	repo  domain.TenantDomainsRepository
	audit *AuditService
}

func NewTenantDomainService(repo domain.TenantDomainsRepository, audit *AuditService) *TenantDomainService {
	return &TenantDomainService{
		repo:  repo,
		audit: audit,
	}
}

func (s *TenantDomainService) List(ctx context.Context, tenantID uuid.UUID) ([]*entities.TenantDomain, error) {
	return s.repo.ListByTenantID(ctx, tenantID)
}

func (s *TenantDomainService) Create(ctx context.Context, tenantID uuid.UUID, hostname string) (*entities.TenantDomain, error) {
	normalized, err := normalizeHostname(hostname)
	if err != nil {
		return nil, errors.Wrap(ErrTenantDomainInvalidHostname, err.Error())
	}

	token, err := randomHex(32)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate verification token")
	}

	row := &entities.TenantDomain{
		TenantID:          tenantID,
		Hostname:          normalized,
		VerificationToken: token,
	}

	var created *entities.TenantDomain
	err = composables.InTx(composables.WithTenantID(ctx, tenantID), func(txCtx context.Context) error {
		d, err := s.repo.Create(txCtx, row)
		if err != nil {
			var pgErr *pgconn.PgError
			if stderrors.As(err, &pgErr) && pgErr.Code == "23505" {
				return ErrTenantDomainHostnameTaken
			}
			return err
		}
		created = d

		if s.audit != nil {
			_ = s.audit.Log(txCtx, &tenantID, "tenant.domains.add", map[string]any{
				"domain": d,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *TenantDomainService) Verify(ctx context.Context, tenantID, domainID uuid.UUID) (*entities.TenantDomain, bool, error) {
	d, err := s.repo.GetByID(ctx, domainID)
	if err != nil {
		return nil, false, ErrTenantDomainNotFound
	}
	if d.TenantID != tenantID {
		return nil, false, ErrTenantDomainNotFound
	}

	ok, verifyErr := verifyDomainTXT(ctx, d.Hostname, d.VerificationToken)
	attemptedAt := time.Now()

	var verifiedAt *time.Time
	var lastErr *string
	if ok {
		verifiedAt = &attemptedAt
	} else {
		msg := "TXT record not found"
		if verifyErr != nil {
			msg = verifyErr.Error()
		}
		lastErr = &msg
	}

	var updated *entities.TenantDomain
	err = composables.InTx(composables.WithTenantID(ctx, tenantID), func(txCtx context.Context) error {
		u, err := s.repo.UpdateVerification(txCtx, domainID, attemptedAt, verifiedAt, lastErr)
		if err != nil {
			return err
		}
		updated = u

		if s.audit != nil {
			_ = s.audit.Log(txCtx, &tenantID, "tenant.domains.verify", map[string]any{
				"domain_id": domainID.String(),
				"hostname":  d.Hostname,
				"ok":        ok,
				"error":     lastErr,
			})
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return updated, ok, nil
}

func (s *TenantDomainService) MakePrimary(ctx context.Context, tenantID, domainID uuid.UUID) error {
	d, err := s.repo.GetByID(ctx, domainID)
	if err != nil {
		return ErrTenantDomainNotFound
	}
	if d.TenantID != tenantID {
		return ErrTenantDomainNotFound
	}
	if d.VerifiedAt == nil {
		return ErrTenantDomainNotVerified
	}

	return composables.InTx(composables.WithTenantID(ctx, tenantID), func(txCtx context.Context) error {
		if err := s.repo.SetPrimary(txCtx, tenantID, domainID); err != nil {
			return err
		}
		if s.audit != nil {
			_ = s.audit.Log(txCtx, &tenantID, "tenant.domains.make_primary", map[string]any{
				"domain": d,
			})
		}
		return nil
	})
}

func (s *TenantDomainService) Delete(ctx context.Context, tenantID, domainID uuid.UUID) error {
	d, err := s.repo.GetByID(ctx, domainID)
	if err != nil {
		return ErrTenantDomainNotFound
	}
	if d.TenantID != tenantID {
		return ErrTenantDomainNotFound
	}
	if d.IsPrimary {
		return ErrTenantDomainIsPrimary
	}

	return composables.InTx(composables.WithTenantID(ctx, tenantID), func(txCtx context.Context) error {
		if err := s.repo.Delete(txCtx, tenantID, domainID); err != nil {
			return err
		}
		if s.audit != nil {
			_ = s.audit.Log(txCtx, &tenantID, "tenant.domains.delete", map[string]any{
				"domain": d,
			})
		}
		return nil
	})
}

func normalizeHostname(raw string) (string, error) {
	host := strings.ToLower(strings.TrimSpace(raw))
	if host == "" {
		return "", errors.New("hostname is required")
	}
	if strings.Contains(host, "://") {
		return "", errors.New("scheme is not allowed")
	}
	if strings.Contains(host, "/") {
		return "", errors.New("path is not allowed")
	}
	if strings.Contains(host, ":") {
		return "", errors.New("port is not allowed")
	}
	if strings.ContainsAny(host, " \t\r\n") {
		return "", errors.New("whitespace is not allowed")
	}
	if len(host) > 255 {
		return "", errors.New("hostname too long")
	}
	return host, nil
}

func randomHex(n int) (string, error) {
	if n <= 0 {
		return "", fmt.Errorf("invalid random length: %d", n)
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	const hexdigits = "0123456789abcdef"
	out := make([]byte, 0, len(buf)*2)
	for _, b := range buf {
		out = append(out, hexdigits[b>>4], hexdigits[b&0x0f])
	}
	return string(out), nil
}

func verifyDomainTXT(ctx context.Context, hostname string, token string) (bool, error) {
	lookupName := "_iota-verify." + hostname
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	txts, err := net.DefaultResolver.LookupTXT(lookupCtx, lookupName)
	if err != nil {
		return false, err
	}
	for _, v := range txts {
		if strings.TrimSpace(v) == token {
			return true, nil
		}
	}
	return false, nil
}
