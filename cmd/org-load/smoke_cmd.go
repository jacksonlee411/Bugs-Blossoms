package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type smokeOptions struct {
	BaseURL  string
	TenantID string
	SID      string
}

func newSmokeCmd() *cobra.Command {
	var opts smokeOptions

	cmd := &cobra.Command{
		Use:   "smoke --base-url <url> --tenant <uuid> --sid <cookie>",
		Short: "Run a small smoke check against /health and Org API",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(opts.BaseURL) == "" {
				return errors.New("--base-url is required")
			}
			if strings.TrimSpace(opts.TenantID) == "" {
				return errors.New("--tenant is required")
			}
			if strings.TrimSpace(opts.SID) == "" {
				return errors.New("--sid is required")
			}

			client := newHTTPClient()
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()

			if err := smokeCheck(ctx, client, opts.BaseURL); err != nil {
				return err
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(opts.BaseURL, "/")+"/org/api/hierarchies?type=OrgUnit", nil)
			if err != nil {
				return err
			}
			req.AddCookie(&http.Cookie{Name: "sid", Value: opts.SID})
			resp, err := client.Do(req)
			if err != nil {
				return err
			}
			_ = resp.Body.Close()
			if resp.StatusCode/100 != 2 {
				return fmt.Errorf("org smoke failed: status=%d", resp.StatusCode)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&opts.BaseURL, "base-url", "http://localhost:3200", "server base URL")
	cmd.Flags().StringVar(&opts.TenantID, "tenant", "", "tenant UUID (for required flag parity)")
	cmd.Flags().StringVar(&opts.SID, "sid", "", "session cookie value (sid)")

	return cmd
}
