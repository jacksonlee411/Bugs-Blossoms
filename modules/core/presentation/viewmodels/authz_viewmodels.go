package viewmodels

// AuthzChangesSummary represents the summary of staged changes in the workspace.
type AuthzChangesSummary struct {
	Added     int
	Removed   int
	Resources []string
}

type AuthzWorkspacePreviewItem struct {
	StageKind string
	Type      string
	Domain    string
	Object    string
	Action    string
	Effect    string
}

type AuthzWorkspacePreview struct {
	Items         []AuthzWorkspacePreviewItem
	Domains       []string
	DenyCount     int
	WildcardCount int
}

// PolicyDiffItem describes a single diff operation for display.
type PolicyDiffItem struct {
	Op   string
	Path string
	Text string
}

type AuthzPolicySuggestionItem struct {
	Subject string
	Domain  string
	Object  string
	Action  string
	Effect  string
}
