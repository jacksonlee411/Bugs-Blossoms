package authorization

import "github.com/iota-uz/iota-sdk/pkg/authz"

type RequestFormProps struct {
	RequestURL   string
	Object       string
	Action       string
	Domain       string
	Subject      string
	BaseRevision string
	RequestID    string
	Diff         string
	Reason       string
	SubmitLabel  string
}

type UnauthorizedProps struct {
	State         *authz.ViewState
	Object        string
	Action        string
	Operation     string
	RequestURL    string
	Subject       string
	Domain        string
	DebugURL      string
	BaseRevision  string
	RequestID     string
	Reason        string
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
	RequestURL   string
	BaseRevision string
	RequestID    string
	Reason       string
	CanDebug     bool
}
