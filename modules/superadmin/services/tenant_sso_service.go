package services

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
	ErrSSOInvalidProtocol     = errors.New("invalid protocol")
	ErrSSOInvalidConnectionID = errors.New("invalid connection_id")
	ErrSSOConnectionNotFound  = errors.New("sso connection not found")
	ErrSSOTestFailed          = errors.New("sso connection test failed")
)

type TenantSSOUpsertInput struct {
	ConnectionID        string
	DisplayName         string
	Protocol            string
	JacksonBaseURL      string
	KratosProviderID    string
	SAMLMetadataURL     string
	SAMLMetadataXML     string
	OIDCIssuer          string
	OIDCClientID        string
	OIDCClientSecretRef string
}

type TenantSSOService struct {
	repo       domain.TenantSSOConnectionsRepository
	audit      *AuditService
	httpClient *http.Client
}

func NewTenantSSOService(repo domain.TenantSSOConnectionsRepository, audit *AuditService) *TenantSSOService {
	return &TenantSSOService{
		repo:  repo,
		audit: audit,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (s *TenantSSOService) List(ctx context.Context, tenantID uuid.UUID) ([]*entities.TenantSSOConnection, error) {
	return s.repo.ListByTenantID(ctx, tenantID)
}

func (s *TenantSSOService) Get(ctx context.Context, tenantID, connID uuid.UUID) (*entities.TenantSSOConnection, error) {
	conn, err := s.repo.GetByID(ctx, connID)
	if err != nil {
		return nil, ErrSSOConnectionNotFound
	}
	if conn.TenantID != tenantID {
		return nil, ErrSSOConnectionNotFound
	}
	return conn, nil
}

func (s *TenantSSOService) Create(ctx context.Context, tenantID uuid.UUID, in TenantSSOUpsertInput) (*entities.TenantSSOConnection, error) {
	row, err := s.buildEntity(tenantID, uuid.Nil, in)
	if err != nil {
		return nil, err
	}
	row.Enabled = false

	var created *entities.TenantSSOConnection
	err = composables.InTx(composables.WithTenantID(ctx, tenantID), func(txCtx context.Context) error {
		c, err := s.repo.Create(txCtx, row)
		if err != nil {
			var pgErr *pgconn.PgError
			if stderrors.As(err, &pgErr) && pgErr.Code == "23505" {
				return errors.New("connection_id already exists")
			}
			return err
		}
		created = c

		if s.audit != nil {
			_ = s.audit.Log(txCtx, &tenantID, "tenant.sso.create", map[string]any{
				"connection": created,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *TenantSSOService) Update(ctx context.Context, tenantID, connID uuid.UUID, in TenantSSOUpsertInput) (*entities.TenantSSOConnection, error) {
	row, err := s.buildEntity(tenantID, connID, in)
	if err != nil {
		return nil, err
	}

	var updated *entities.TenantSSOConnection
	err = composables.InTx(composables.WithTenantID(ctx, tenantID), func(txCtx context.Context) error {
		c, err := s.repo.Update(txCtx, row)
		if err != nil {
			return err
		}

		c, err = s.repo.UpdateStatus(txCtx, c.ID, false, nil, nil, nil)
		if err != nil {
			return err
		}
		updated = c

		if s.audit != nil {
			_ = s.audit.Log(txCtx, &tenantID, "tenant.sso.update", map[string]any{
				"connection": updated,
				"note":       "connection disabled and test status cleared",
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *TenantSSOService) Test(ctx context.Context, tenantID, connID uuid.UUID) (*entities.TenantSSOConnection, bool, error) {
	conn, err := s.repo.GetByID(ctx, connID)
	if err != nil {
		return nil, false, ErrSSOConnectionNotFound
	}
	if conn.TenantID != tenantID {
		return nil, false, ErrSSOConnectionNotFound
	}

	status, lastErr := s.runConnectionTest(ctx, conn)
	now := time.Now()
	ok := status == "ok"

	var updated *entities.TenantSSOConnection
	err = composables.InTx(composables.WithTenantID(ctx, tenantID), func(txCtx context.Context) error {
		c, err := s.repo.UpdateStatus(txCtx, connID, conn.Enabled, &status, lastErr, &now)
		if err != nil {
			return err
		}
		updated = c

		if s.audit != nil {
			_ = s.audit.Log(txCtx, &tenantID, "tenant.sso.test", map[string]any{
				"connection_id": connID.String(),
				"status":        status,
				"error":         lastErr,
			})
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return updated, ok, nil
}

func (s *TenantSSOService) Enable(ctx context.Context, tenantID, connID uuid.UUID) (*entities.TenantSSOConnection, error) {
	conn, err := s.repo.GetByID(ctx, connID)
	if err != nil {
		return nil, ErrSSOConnectionNotFound
	}
	if conn.TenantID != tenantID {
		return nil, ErrSSOConnectionNotFound
	}

	status, lastErr := s.runConnectionTest(ctx, conn)
	now := time.Now()
	if status != "ok" {
		_, _ = s.updateTestAndEnabled(ctx, tenantID, connID, false, &status, lastErr, &now, "tenant.sso.enable_failed")
		return nil, ErrSSOTestFailed
	}
	return s.updateTestAndEnabled(ctx, tenantID, connID, true, &status, lastErr, &now, "tenant.sso.enable")
}

func (s *TenantSSOService) Disable(ctx context.Context, tenantID, connID uuid.UUID) (*entities.TenantSSOConnection, error) {
	conn, err := s.repo.GetByID(ctx, connID)
	if err != nil {
		return nil, ErrSSOConnectionNotFound
	}
	if conn.TenantID != tenantID {
		return nil, ErrSSOConnectionNotFound
	}

	var updated *entities.TenantSSOConnection
	err = composables.InTx(composables.WithTenantID(ctx, tenantID), func(txCtx context.Context) error {
		c, err := s.repo.UpdateStatus(txCtx, connID, false, conn.LastTestStatus, conn.LastTestError, conn.LastTestAt)
		if err != nil {
			return err
		}
		updated = c

		if s.audit != nil {
			_ = s.audit.Log(txCtx, &tenantID, "tenant.sso.disable", map[string]any{
				"connection_id": connID.String(),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *TenantSSOService) Delete(ctx context.Context, tenantID, connID uuid.UUID) error {
	conn, err := s.repo.GetByID(ctx, connID)
	if err != nil {
		return ErrSSOConnectionNotFound
	}
	if conn.TenantID != tenantID {
		return ErrSSOConnectionNotFound
	}

	return composables.InTx(composables.WithTenantID(ctx, tenantID), func(txCtx context.Context) error {
		if err := s.repo.Delete(txCtx, tenantID, connID); err != nil {
			return err
		}
		if s.audit != nil {
			_ = s.audit.Log(txCtx, &tenantID, "tenant.sso.delete", map[string]any{
				"connection": conn,
			})
		}
		return nil
	})
}

func (s *TenantSSOService) updateTestAndEnabled(ctx context.Context, tenantID, connID uuid.UUID, enabled bool, status, lastErr *string, at *time.Time, action string) (*entities.TenantSSOConnection, error) {
	var updated *entities.TenantSSOConnection
	err := composables.InTx(composables.WithTenantID(ctx, tenantID), func(txCtx context.Context) error {
		c, err := s.repo.UpdateStatus(txCtx, connID, enabled, status, lastErr, at)
		if err != nil {
			return err
		}
		updated = c
		if s.audit != nil {
			_ = s.audit.Log(txCtx, &tenantID, action, map[string]any{
				"connection_id": connID.String(),
				"enabled":       enabled,
				"status":        status,
				"error":         lastErr,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *TenantSSOService) buildEntity(tenantID uuid.UUID, id uuid.UUID, in TenantSSOUpsertInput) (*entities.TenantSSOConnection, error) {
	connectionID := strings.ToLower(strings.TrimSpace(in.ConnectionID))
	if !isValidConnectionID(connectionID) {
		return nil, ErrSSOInvalidConnectionID
	}

	displayName := strings.TrimSpace(in.DisplayName)
	if displayName == "" {
		return nil, errors.New("display_name is required")
	}

	protocol := strings.ToLower(strings.TrimSpace(in.Protocol))
	if protocol != "saml" && protocol != "oidc" {
		return nil, ErrSSOInvalidProtocol
	}

	jacksonBaseURL := strings.TrimSpace(in.JacksonBaseURL)
	if jacksonBaseURL == "" {
		return nil, errors.New("jackson_base_url is required")
	}
	if _, err := url.ParseRequestURI(jacksonBaseURL); err != nil {
		return nil, errors.New("invalid jackson_base_url")
	}

	kratosProviderID := strings.TrimSpace(in.KratosProviderID)
	if kratosProviderID == "" {
		return nil, errors.New("kratos_provider_id is required")
	}

	row := &entities.TenantSSOConnection{
		ID:               id,
		TenantID:         tenantID,
		ConnectionID:     connectionID,
		DisplayName:      displayName,
		Protocol:         protocol,
		JacksonBaseURL:   jacksonBaseURL,
		KratosProviderID: kratosProviderID,
	}

	if protocol == "saml" {
		metadataURL := strings.TrimSpace(in.SAMLMetadataURL)
		metadataXML := strings.TrimSpace(in.SAMLMetadataXML)
		if metadataURL == "" && metadataXML == "" {
			return nil, errors.New("saml requires metadata_url or metadata_xml")
		}
		if metadataURL != "" {
			if _, err := url.ParseRequestURI(metadataURL); err != nil {
				return nil, errors.New("invalid saml_metadata_url")
			}
			row.SAMLMetadataURL = &metadataURL
		}
		if metadataXML != "" {
			row.SAMLMetadataXML = &metadataXML
		}
	}

	if protocol == "oidc" {
		issuer := strings.TrimSpace(in.OIDCIssuer)
		clientID := strings.TrimSpace(in.OIDCClientID)
		secretRef := strings.TrimSpace(in.OIDCClientSecretRef)
		if issuer == "" || clientID == "" || secretRef == "" {
			return nil, errors.New("oidc requires issuer, client_id and client_secret_ref")
		}
		if _, err := url.ParseRequestURI(issuer); err != nil {
			return nil, errors.New("invalid oidc_issuer")
		}
		if err := ValidateSecretRefFormat(secretRef); err != nil {
			return nil, fmt.Errorf("invalid client_secret_ref format: %w", err)
		}
		row.OIDCIssuer = &issuer
		row.OIDCClientID = &clientID
		row.OIDCClientSecretRef = &secretRef
	}

	return row, nil
}

func (s *TenantSSOService) runConnectionTest(ctx context.Context, conn *entities.TenantSSOConnection) (string, *string) {
	switch conn.Protocol {
	case "oidc":
		if conn.OIDCIssuer == nil || conn.OIDCClientID == nil || conn.OIDCClientSecretRef == nil {
			msg := "oidc config incomplete"
			return "failed", &msg
		}
		if _, err := ResolveSecretRef(*conn.OIDCClientSecretRef); err != nil {
			msg := fmt.Sprintf("secret_ref resolve failed: %v", err)
			return "failed", &msg
		}
		if err := s.testOIDC(ctx, *conn.OIDCIssuer); err != nil {
			msg := err.Error()
			return "failed", &msg
		}
		return "ok", nil
	case "saml":
		if conn.SAMLMetadataURL == nil && conn.SAMLMetadataXML == nil {
			msg := "saml metadata is required"
			return "failed", &msg
		}
		if conn.SAMLMetadataURL != nil {
			if err := s.testSAMLMetadataURL(ctx, *conn.SAMLMetadataURL); err != nil {
				msg := err.Error()
				return "failed", &msg
			}
		}
		if conn.SAMLMetadataXML != nil {
			if err := testSAMLMetadataXML(*conn.SAMLMetadataXML); err != nil {
				msg := err.Error()
				return "failed", &msg
			}
		}
		return "ok", nil
	default:
		msg := "unknown protocol"
		return "failed", &msg
	}
}

func isValidConnectionID(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			continue
		}
		return false
	}
	if s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	if strings.Contains(s, "--") {
		return false
	}
	return true
}

type oidcWellKnown struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

func (s *TenantSSOService) testOIDC(ctx context.Context, issuer string) error {
	issuer = strings.TrimSpace(issuer)
	wellKnownURL := strings.TrimSuffix(issuer, "/") + "/.well-known/openid-configuration"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnownURL, nil)
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return fmt.Errorf("well-known request failed: %s (%s)", resp.Status, strings.TrimSpace(string(b)))
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return err
	}

	var wk oidcWellKnown
	if err := json.Unmarshal(b, &wk); err != nil {
		return errors.Wrap(err, "invalid well-known json")
	}
	if wk.Issuer == "" || wk.AuthorizationEndpoint == "" || wk.TokenEndpoint == "" {
		return errors.New("well-known missing required fields")
	}
	if strings.TrimSuffix(wk.Issuer, "/") != strings.TrimSuffix(issuer, "/") {
		return fmt.Errorf("issuer mismatch: expected %q got %q", issuer, wk.Issuer)
	}
	return nil
}

func (s *TenantSSOService) testSAMLMetadataURL(ctx context.Context, metadataURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("metadata request failed: %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return err
	}
	return testSAMLMetadataXML(string(b))
}

func testSAMLMetadataXML(xml string) error {
	xml = strings.TrimSpace(xml)
	if xml == "" {
		return errors.New("metadata xml is empty")
	}
	if !strings.Contains(xml, "EntityDescriptor") && !strings.Contains(xml, "EntitiesDescriptor") {
		return errors.New("metadata xml does not look like SAML metadata")
	}
	return nil
}
