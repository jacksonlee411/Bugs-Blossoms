package authorization

import "github.com/iota-uz/iota-sdk/pkg/authz"

type UnauthorizedProps struct {
	State         *authz.ViewState
	Object        string
	Action        string
	Operation     string
	Subject       string
	Domain        string
	DebugURL      string
	BaseRevision  string
	RequestID     string
	ShowInspector bool
	CanDebug      bool
}

type PolicyInspectorProps struct {
	State        *authz.ViewState
	Object       string
	Action       string
	Subject      string
	Domain       string
	DebugURL     string
	BaseRevision string
	RequestID    string
	CanDebug     bool
}
