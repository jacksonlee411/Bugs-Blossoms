//go:build tools

package tools

import (
	_ "github.com/99designs/gqlgen"
	_ "github.com/99designs/gqlgen/graphql/introspection"
	_ "github.com/roblaszczak/go-cleanarch"
	_ "github.com/sqlc-dev/sqlc/cmd/sqlc"
)
