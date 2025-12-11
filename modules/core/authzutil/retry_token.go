package authzutil

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

var (
	ErrInvalidRetryToken = errors.New("invalid retry token")
	ErrExpiredRetryToken = errors.New("retry token expired")
)

type retryTokenPayload struct {
	RequestID string `json:"request_id"`
	ExpiresAt int64  `json:"exp"`
	Nonce     string `json:"nonce"`
}

// GenerateRetryToken issues a short-lived, self-contained token without storing server-side state.
func GenerateRetryToken(requestID uuid.UUID, ttl time.Duration) (string, error) {
	if requestID == uuid.Nil {
		return "", ErrInvalidRetryToken
	}
	secret := strings.TrimSpace(configuration.Use().SidCookieKey)
	if secret == "" {
		return "", fmt.Errorf("sid cookie key is empty")
	}
	payload := retryTokenPayload{
		RequestID: requestID.String(),
		ExpiresAt: time.Now().Add(ttl).Unix(),
		Nonce:     uuid.NewString(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write([]byte(encoded)); err != nil {
		return "", err
	}
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return encoded + "." + signature, nil
}

// ValidateRetryToken checks signature, expiration and request id binding.
func ValidateRetryToken(token string, requestID uuid.UUID) error {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return ErrInvalidRetryToken
	}
	secret := strings.TrimSpace(configuration.Use().SidCookieKey)
	if secret == "" {
		return fmt.Errorf("sid cookie key is empty")
	}
	payload := parts[0]
	expectedMAC := hmac.New(sha256.New, []byte(secret))
	if _, err := expectedMAC.Write([]byte(payload)); err != nil {
		return err
	}
	expectedSig := base64.RawURLEncoding.EncodeToString(expectedMAC.Sum(nil))
	if !hmac.Equal([]byte(expectedSig), []byte(parts[1])) {
		return ErrInvalidRetryToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return ErrInvalidRetryToken
	}
	var parsed retryTokenPayload
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ErrInvalidRetryToken
	}
	if parsed.RequestID != requestID.String() {
		return ErrInvalidRetryToken
	}
	if time.Unix(parsed.ExpiresAt, 0).Before(time.Now()) {
		return ErrExpiredRetryToken
	}
	return nil
}
