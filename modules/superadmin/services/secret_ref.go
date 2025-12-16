package services

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

var (
	ErrSecretRefEmpty             = errors.New("secret_ref is empty")
	ErrSecretRefUnsupportedScheme = errors.New("unsupported secret_ref scheme")
	ErrSecretRefNotFound          = errors.New("secret_ref not found")
	ErrSecretRefInvalidValue      = errors.New("secret_ref resolved to invalid value")
)

func ValidateSecretRefFormat(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ErrSecretRefEmpty
	}

	switch {
	case strings.HasPrefix(ref, "ENV:"):
		name := strings.TrimSpace(strings.TrimPrefix(ref, "ENV:"))
		if name == "" {
			return ErrSecretRefEmpty
		}
		return nil
	case strings.HasPrefix(ref, "FILE:"):
		path := strings.TrimSpace(strings.TrimPrefix(ref, "FILE:"))
		if path == "" {
			return ErrSecretRefEmpty
		}
		if !filepath.IsAbs(path) {
			return errors.New("FILE: secret_ref must be an absolute path")
		}
		return nil
	default:
		return ErrSecretRefUnsupportedScheme
	}
}

func ResolveSecretRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", ErrSecretRefEmpty
	}

	switch {
	case strings.HasPrefix(ref, "ENV:"):
		name := strings.TrimSpace(strings.TrimPrefix(ref, "ENV:"))
		if name == "" {
			return "", ErrSecretRefEmpty
		}
		v, ok := os.LookupEnv(name)
		if !ok {
			return "", ErrSecretRefNotFound
		}
		return validateSecretValue(v)
	case strings.HasPrefix(ref, "FILE:"):
		path := strings.TrimSpace(strings.TrimPrefix(ref, "FILE:"))
		if path == "" {
			return "", ErrSecretRefEmpty
		}
		if !filepath.IsAbs(path) {
			return "", errors.New("FILE: secret_ref must be an absolute path")
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return "", errors.Wrap(err, "failed to read secret_ref file")
		}
		return validateSecretValue(string(b))
	default:
		return "", ErrSecretRefUnsupportedScheme
	}
}

func SecretRefStatus(ref *string) (string, error) {
	if ref == nil || strings.TrimSpace(*ref) == "" {
		return "missing", ErrSecretRefEmpty
	}
	_, err := ResolveSecretRef(*ref)
	if err != nil {
		return "error", err
	}
	return "ok", nil
}

func validateSecretValue(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", ErrSecretRefInvalidValue
	}
	if strings.ContainsAny(v, "\n\r") {
		return "", ErrSecretRefInvalidValue
	}
	return v, nil
}
