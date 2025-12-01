package controllers_test

import (
	"context"
	"os"
	"testing"

	"github.com/iota-uz/go-i18n/v2/i18n"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/intl"
)

func maybeEnableParallel(t *testing.T) {
	t.Helper()
	if os.Getenv("SUPERADMIN_TEST_PARALLEL") == "1" {
		t.Parallel()
	}
}

func ctxWithEnglishLocalizer(ctx context.Context, app application.Application) context.Context {
	tl := i18n.NewLocalizer(app.Bundle(), "en")
	return intl.WithLocalizer(ctx, tl)
}
