package services

import "github.com/google/uuid"

type OrgNodeAsOfRow struct {
	ID       uuid.UUID
	ParentID *uuid.UUID
	Code     string
	Name     string
	Status   string
}

type OrgLinkSummary struct {
	ObjectType string `json:"object_type"`
	ObjectKey  string `json:"object_key"`
	LinkType   string `json:"link_type"`
}
