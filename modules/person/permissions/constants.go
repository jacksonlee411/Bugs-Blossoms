package permissions

import (
	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/core/domain/entities/permission"
)

const (
	ResourcePerson permission.Resource = "person"
)

var (
	PersonCreate = &permission.Permission{
		ID:       uuid.MustParse("06b37b05-9df7-47c6-8c0d-59fe9c1321b7"),
		Name:     "Person.Create",
		Resource: ResourcePerson,
		Action:   permission.ActionCreate,
		Modifier: permission.ModifierAll,
	}
	PersonRead = &permission.Permission{
		ID:       uuid.MustParse("23f8920c-6eba-4f9f-854c-664bfb4e8a69"),
		Name:     "Person.Read",
		Resource: ResourcePerson,
		Action:   permission.ActionRead,
		Modifier: permission.ModifierAll,
	}
	PersonUpdate = &permission.Permission{
		ID:       uuid.MustParse("f59b0586-8c1a-401f-ad6c-ea1e5871f89a"),
		Name:     "Person.Update",
		Resource: ResourcePerson,
		Action:   permission.ActionUpdate,
		Modifier: permission.ModifierAll,
	}
	PersonDelete = &permission.Permission{
		ID:       uuid.MustParse("188e63a2-003b-4878-9c22-3d872b2e185b"),
		Name:     "Person.Delete",
		Resource: ResourcePerson,
		Action:   permission.ActionDelete,
		Modifier: permission.ModifierAll,
	}
)

var Permissions = []*permission.Permission{
	PersonCreate,
	PersonRead,
	PersonUpdate,
	PersonDelete,
}
