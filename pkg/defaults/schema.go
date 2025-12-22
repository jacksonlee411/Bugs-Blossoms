package defaults

import (
	"github.com/iota-uz/iota-sdk/modules/core/domain/entities/permission"
	"github.com/iota-uz/iota-sdk/pkg/rbac"

	corePerms "github.com/iota-uz/iota-sdk/modules/core/permissions"
	loggingPerms "github.com/iota-uz/iota-sdk/modules/logging/permissions"
	personPerms "github.com/iota-uz/iota-sdk/modules/person/permissions"
)

// permissionSetBuilder helps create consistent permission sets
type permissionSetBuilder struct {
	module string
	prefix string
}

// newPermissionSetBuilder creates a new builder for a module
func newPermissionSetBuilder(module string) *permissionSetBuilder {
	return &permissionSetBuilder{
		module: module,
		prefix: "PermissionSets." + module + ".",
	}
}

// viewSet creates a "view" permission set for a resource
func (b *permissionSetBuilder) viewSet(resource string, readPerm *permission.Permission) rbac.PermissionSet {
	return rbac.PermissionSet{
		Key:         resource + "_view",
		Label:       b.prefix + resource + "View.Label",
		Description: b.prefix + resource + "View._Description",
		Module:      b.module,
		Permissions: []*permission.Permission{readPerm},
	}
}

// manageSet creates a "manage" permission set for a resource with full CRUD permissions
func (b *permissionSetBuilder) manageSet(resource string, create, read, update, deletePerm *permission.Permission) rbac.PermissionSet {
	return rbac.PermissionSet{
		Key:         resource + "_manage",
		Label:       b.prefix + resource + "Manage.Label",
		Description: b.prefix + resource + "Manage._Description",
		Module:      b.module,
		Permissions: []*permission.Permission{create, read, update, deletePerm},
	}
}

// AllPermissions returns all permissions from all modules
// This is used for seeding and RBAC initialization
func AllPermissions() []*permission.Permission {
	// Pre-calculate total capacity to avoid slice re-allocations
	totalCapacity := len(corePerms.Permissions) +
		len(personPerms.Permissions) +
		len(loggingPerms.Permissions)

	permissions := make([]*permission.Permission, 0, totalCapacity)
	permissions = append(permissions, corePerms.Permissions...)
	permissions = append(permissions, personPerms.Permissions...)
	permissions = append(permissions, loggingPerms.Permissions...)
	return permissions
}

// PermissionSchema returns the default permission schema with grouped permissions
func PermissionSchema() *rbac.PermissionSchema {
	sets := buildModulePermissionSets()

	// Add remaining modules as individual permission sets for now
	sets = appendRemainingPermissionSets(sets)

	return &rbac.PermissionSchema{Sets: sets}
}

// buildModulePermissionSets creates permission sets for all modules using the builder pattern
func buildModulePermissionSets() []rbac.PermissionSet {
	var sets []rbac.PermissionSet

	// Core module
	core := newPermissionSetBuilder("Core")
	sets = append(sets,
		core.viewSet("User", corePerms.UserRead),
		core.manageSet("User", corePerms.UserCreate, corePerms.UserRead, corePerms.UserUpdate, corePerms.UserDelete),
		core.viewSet("Role", corePerms.RoleRead),
		core.manageSet("Role", corePerms.RoleCreate, corePerms.RoleRead, corePerms.RoleUpdate, corePerms.RoleDelete),
		core.viewSet("Group", corePerms.GroupRead),
		core.manageSet("Group", corePerms.GroupCreate, corePerms.GroupRead, corePerms.GroupUpdate, corePerms.GroupDelete),
		core.viewSet("Upload", corePerms.UploadRead),
		core.manageSet("Upload", corePerms.UploadCreate, corePerms.UploadRead, corePerms.UploadUpdate, corePerms.UploadDelete),
		core.viewSet("AuthzRequests", corePerms.AuthzRequestsRead),
		core.manageSet("AuthzRequests", corePerms.AuthzRequestsWrite, corePerms.AuthzRequestsRead, corePerms.AuthzRequestsReview, corePerms.AuthzRequestsDelete),
		core.viewSet("AuthzDebug", corePerms.AuthzDebug),
	)

	// Person module
	person := newPermissionSetBuilder("Person")
	sets = append(sets,
		person.viewSet("Person", personPerms.PersonRead),
		person.manageSet("Person", personPerms.PersonCreate, personPerms.PersonRead, personPerms.PersonUpdate, personPerms.PersonDelete),
	)

	// Logging module
	logging := newPermissionSetBuilder("Logging")
	sets = append(sets,
		logging.viewSet("Logs", loggingPerms.ViewLogs),
	)

	return sets
}

// appendRemainingPermissionSets adds remaining modules as individual permission sets
func appendRemainingPermissionSets(sets []rbac.PermissionSet) []rbac.PermissionSet {
	return sets
}
