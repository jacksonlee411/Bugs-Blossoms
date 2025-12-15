package outbox

import (
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
)

var identPartRe = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// ParseIdentifier parses "schema.table" or "table" into pgx.Identifier.
func ParseIdentifier(s string) (pgx.Identifier, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, invalidConfig("identifier is empty")
	}

	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return nil, invalidConfig("invalid identifier %q (expected table or schema.table)", s)
	}

	ident := make(pgx.Identifier, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, invalidConfig("invalid identifier %q (empty part)", s)
		}
		if !identPartRe.MatchString(p) {
			return nil, invalidConfig("invalid identifier %q (bad part %q)", s, p)
		}
		ident = append(ident, p)
	}
	return ident, nil
}

// ParseIdentifierList parses comma-separated identifiers.
func ParseIdentifierList(s string) ([]pgx.Identifier, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}

	raw := strings.Split(s, ",")
	out := make([]pgx.Identifier, 0, len(raw))
	for _, item := range raw {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		ident, err := ParseIdentifier(item)
		if err != nil {
			return nil, err
		}
		out = append(out, ident)
	}
	return out, nil
}
