package composables

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/iota-uz/iota-sdk/pkg/configuration"
)

func ApplyTenantRLS(ctx context.Context, tx pgx.Tx) error {
	if configuration.Use().RLSEnforce != "enforce" {
		return nil
	}
	tenantID, err := UseTenantID(ctx)
	if err != nil {
		return fmt.Errorf("rls requires tenant in context: %w", err)
	}
	_, err = tx.Exec(ctx, "SELECT set_config('app.current_tenant', $1, true)", tenantID.String())
	if err != nil {
		return fmt.Errorf("failed to set rls tenant context: %w", err)
	}
	return nil
}
