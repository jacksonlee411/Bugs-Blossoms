package authzutil

import (
	"context"
	"sync"

	authzVersion "github.com/iota-uz/iota-sdk/pkg/authz/version"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

var (
	revisionOnce sync.Once
	revisionProv *authzVersion.FileProvider
)

// BaseRevision returns the current aggregated policy revision string for UI rendering.
func BaseRevision(ctx context.Context) string {
	revisionOnce.Do(func() {
		cfg := configuration.Use()
		path := cfg.Authz.PolicyPath + ".rev"
		revisionProv = authzVersion.NewFileProvider(path)
	})
	if revisionProv == nil {
		return ""
	}
	meta, err := revisionProv.Current(ctx)
	if err != nil {
		return ""
	}
	return meta.Revision
}
