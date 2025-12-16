package modules

import (
	"slices"

	"github.com/iota-uz/iota-sdk/modules/bichat"
	"github.com/iota-uz/iota-sdk/modules/core"
	"github.com/iota-uz/iota-sdk/modules/hrm"
	"github.com/iota-uz/iota-sdk/modules/logging"
	"github.com/iota-uz/iota-sdk/modules/testkit"
	"github.com/iota-uz/iota-sdk/modules/warehouse"
	"github.com/iota-uz/iota-sdk/modules/website"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/defaults"
)

var (
	BuiltInModules = []application.Module{
		core.NewModule(&core.ModuleOptions{
			PermissionSchema: defaults.PermissionSchema(),
		}),
		bichat.NewModule(),
		hrm.NewModule(),
		logging.NewModule(),
		warehouse.NewModule(),
		website.NewModule(),
		testkit.NewModule(), // Test endpoints - only active when ENABLE_TEST_ENDPOINTS=true
	}

	NavLinks = slices.Concat(
		core.NavItems,
		bichat.NavItems,
		hrm.NavItems,
		logging.NavItems,
		warehouse.NavItems,
		website.NavItems,
	)
)

func Load(app application.Application, externalModules ...application.Module) error {
	for _, module := range externalModules {
		if err := module.Register(app); err != nil {
			return err
		}
	}
	return nil
}
