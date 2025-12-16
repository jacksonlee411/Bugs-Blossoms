package services

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/pkg/composables"
)

type AuditService struct{}

func NewAuditService() *AuditService {
	return &AuditService{}
}

func (s *AuditService) Log(ctx context.Context, tenantID *uuid.UUID, action string, payload any) error {
	tx, err := composables.UseTx(ctx)
	if err != nil {
		return err
	}

	currentUser, err := composables.UseUser(ctx)
	if err != nil {
		return err
	}

	var actorName *string
	name := strings.TrimSpace(strings.Join([]string{currentUser.FirstName(), currentUser.LastName()}, " "))
	if name != "" {
		actorName = &name
	}

	params, _ := composables.UseParams(ctx)
	var ipAddress *string
	var userAgent *string
	if params != nil {
		if params.IP != "" {
			ipAddress = &params.IP
		}
		if params.UserAgent != "" {
			userAgent = &params.UserAgent
		}
	}

	safePayload := redactAuditPayload(payload)
	payloadBytes, err := json.Marshal(safePayload)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO superadmin_audit_logs (
			actor_user_id,
			actor_email_snapshot,
			actor_name_snapshot,
			tenant_id,
			action,
			payload,
			ip_address,
			user_agent,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
	`,
		int64(currentUser.ID()),
		currentUser.Email().Value(),
		actorName,
		tenantID,
		action,
		payloadBytes,
		ipAddress,
		userAgent,
	)
	return err
}

func redactAuditPayload(payload any) any {
	if payload == nil {
		return map[string]any{}
	}

	switch v := payload.(type) {
	case map[string]any:
		return redactMap(v)
	case []any:
		return redactSlice(v)
	default:
		return payload
	}
}

func redactMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for key, value := range m {
		if isSensitiveKey(key) {
			out[key] = "<redacted>"
			continue
		}

		switch typed := value.(type) {
		case map[string]any:
			out[key] = redactMap(typed)
		case []any:
			out[key] = redactSlice(typed)
		default:
			out[key] = value
		}
	}
	return out
}

func redactSlice(s []any) []any {
	out := make([]any, 0, len(s))
	for _, value := range s {
		switch typed := value.(type) {
		case map[string]any:
			out = append(out, redactMap(typed))
		case []any:
			out = append(out, redactSlice(typed))
		default:
			out = append(out, value)
		}
	}
	return out
}

func isSensitiveKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" {
		return false
	}

	sensitiveSubstrings := []string{
		"password",
		"secret",
		"token",
		"cookie",
	}
	for _, s := range sensitiveSubstrings {
		if strings.Contains(k, s) {
			return true
		}
	}

	switch k {
	case "oidc_client_secret_ref":
		return true
	case "verification_token":
		return true
	}
	return false
}
