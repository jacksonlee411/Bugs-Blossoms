package permissions

import (
	"github.com/google/uuid"

	"github.com/iota-uz/iota-sdk/modules/core/domain/entities/permission"
)

const (
	ResourceUser   permission.Resource = "user"
	ResourceRole   permission.Resource = "role"
	ResourceGroup  permission.Resource = "group"
	ResourceUpload permission.Resource = "upload"
	ResourceAuthz  permission.Resource = "authorization"
)

var (
	UserCreate = &permission.Permission{
		ID:       uuid.MustParse("8b6060b3-af5e-4ae0-b32d-b33695141066"),
		Name:     "User.Create",
		Resource: ResourceUser,
		Action:   permission.ActionCreate,
		Modifier: permission.ModifierAll,
	}
	UserRead = &permission.Permission{
		ID:       uuid.MustParse("13f011c8-1107-4957-ad19-70cfc167a775"),
		Name:     "User.Read",
		Resource: ResourceUser,
		Action:   permission.ActionRead,
		Modifier: permission.ModifierAll,
	}
	UserUpdate = &permission.Permission{
		ID:       uuid.MustParse("1c351fd3-9a2b-40b9-80b1-11ba81e645c8"),
		Name:     "User.Update",
		Resource: ResourceUser,
		Action:   permission.ActionUpdate,
		Modifier: permission.ModifierAll,
	}
	UserDelete = &permission.Permission{
		ID:       uuid.MustParse("547cded3-6754-4a05-aeb0-a38d12ed05ee"),
		Name:     "User.Delete",
		Resource: ResourceUser,
		Action:   permission.ActionDelete,
		Modifier: permission.ModifierAll,
	}
	RoleCreate = &permission.Permission{
		ID:       uuid.MustParse("60f195ed-d373-41c3-a39d-bb7484850840"),
		Name:     "Role.Create",
		Resource: ResourceRole,
		Action:   permission.ActionCreate,
		Modifier: permission.ModifierAll,
	}
	RoleRead = &permission.Permission{
		ID:       uuid.MustParse("51d1025e-11fe-405e-9ab4-88078c28e110"),
		Name:     "Role.Read",
		Resource: ResourceRole,
		Action:   permission.ActionRead,
		Modifier: permission.ModifierAll,
	}
	RoleUpdate = &permission.Permission{
		ID:       uuid.MustParse("ea18e9d1-6ac4-4b2a-861c-cc89d95d7a19"),
		Name:     "Role.Update",
		Resource: ResourceRole,
		Action:   permission.ActionUpdate,
		Modifier: permission.ModifierAll,
	}
	RoleDelete = &permission.Permission{
		ID:       uuid.MustParse("5fcea09b-913e-4bbf-bb00-66586c29e930"),
		Name:     "Role.Delete",
		Resource: ResourceRole,
		Action:   permission.ActionDelete,
		Modifier: permission.ModifierAll,
	}
	GroupCreate = &permission.Permission{
		ID:       uuid.MustParse("7e8f9a0b-1c2d-3e4f-5a6b-7c8d9e0f1a2b"),
		Name:     "Group.Create",
		Resource: ResourceGroup,
		Action:   permission.ActionCreate,
		Modifier: permission.ModifierAll,
	}
	GroupRead = &permission.Permission{
		ID:       uuid.MustParse("8f9a0b1c-2d3e-4f5a-6b7c-8d9e0f1a2b3c"),
		Name:     "Group.Read",
		Resource: ResourceGroup,
		Action:   permission.ActionRead,
		Modifier: permission.ModifierAll,
	}
	GroupUpdate = &permission.Permission{
		ID:       uuid.MustParse("9a0b1c2d-3e4f-5a6b-7c8d-9e0f1a2b3c4d"),
		Name:     "Group.Update",
		Resource: ResourceGroup,
		Action:   permission.ActionUpdate,
		Modifier: permission.ModifierAll,
	}
	GroupDelete = &permission.Permission{
		ID:       uuid.MustParse("a0b1c2d3-4e5f-6a7b-8c9d-0e1f2a3b4c5d"),
		Name:     "Group.Delete",
		Resource: ResourceGroup,
		Action:   permission.ActionDelete,
		Modifier: permission.ModifierAll,
	}
	UploadCreate = &permission.Permission{
		ID:       uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890"),
		Name:     "Upload.Create",
		Resource: ResourceUpload,
		Action:   permission.ActionCreate,
		Modifier: permission.ModifierAll,
	}
	UploadRead = &permission.Permission{
		ID:       uuid.MustParse("b2c3d4e5-f6a7-8901-bcde-f23456789012"),
		Name:     "Upload.Read",
		Resource: ResourceUpload,
		Action:   permission.ActionRead,
		Modifier: permission.ModifierAll,
	}
	UploadUpdate = &permission.Permission{
		ID:       uuid.MustParse("c3d4e5f6-a7b8-9012-cdef-345678901234"),
		Name:     "Upload.Update",
		Resource: ResourceUpload,
		Action:   permission.ActionUpdate,
		Modifier: permission.ModifierAll,
	}
	UploadDelete = &permission.Permission{
		ID:       uuid.MustParse("d4e5f6a7-b8c9-0123-defa-456789012345"),
		Name:     "Upload.Delete",
		Resource: ResourceUpload,
		Action:   permission.ActionDelete,
		Modifier: permission.ModifierAll,
	}
	AuthzRequestsWrite = &permission.Permission{
		ID:       uuid.MustParse("0f65fd1a-0edb-4c38-ae4f-6b4ef95c5e6f"),
		Name:     "Authz.Requests.Write",
		Resource: ResourceAuthz,
		Action:   permission.ActionCreate,
		Modifier: permission.ModifierAll,
	}
	AuthzRequestsRead = &permission.Permission{
		ID:       uuid.MustParse("3ea1a6dd-7b73-4397-9a07-6e4220348452"),
		Name:     "Authz.Requests.Read",
		Resource: ResourceAuthz,
		Action:   permission.ActionRead,
		Modifier: permission.ModifierAll,
	}
	AuthzRequestsReview = &permission.Permission{
		ID:       uuid.MustParse("738cb963-78dd-42fe-b9ee-97f1dbe6cfe5"),
		Name:     "Authz.Requests.Review",
		Resource: ResourceAuthz,
		Action:   permission.ActionUpdate,
		Modifier: permission.ModifierAll,
	}
	AuthzRequestsDelete = &permission.Permission{
		ID:       uuid.MustParse("8c8a4b94-16df-4bd9-b88c-fc1f30f51e78"),
		Name:     "Authz.Requests.Delete",
		Resource: ResourceAuthz,
		Action:   permission.ActionDelete,
		Modifier: permission.ModifierAll,
	}
	AuthzDebug = &permission.Permission{
		ID:       uuid.MustParse("cd5f48da-cc30-4ca7-b6f8-21016fe03186"),
		Name:     "Authz.Debug",
		Resource: ResourceAuthz,
		Action:   permission.ActionRead,
		Modifier: permission.ModifierAll,
	}
)

var Permissions = []*permission.Permission{
	UserCreate,
	UserRead,
	UserUpdate,
	UserDelete,
	RoleCreate,
	RoleRead,
	RoleUpdate,
	RoleDelete,
	GroupCreate,
	GroupRead,
	GroupUpdate,
	GroupDelete,
	UploadCreate,
	UploadRead,
	UploadUpdate,
	UploadDelete,
	AuthzRequestsWrite,
	AuthzRequestsRead,
	AuthzRequestsReview,
	AuthzRequestsDelete,
	AuthzDebug,
}
