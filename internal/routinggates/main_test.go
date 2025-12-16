package routinggates

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("GO_APP_ENV", "production")
	_ = os.Setenv("ENABLE_DEV_ENDPOINTS", "false")
	_ = os.Setenv("ENABLE_GRAPHQL_PLAYGROUND", "false")
	_ = os.Setenv("ENABLE_TEST_ENDPOINTS", "false")

	os.Exit(m.Run())
}
