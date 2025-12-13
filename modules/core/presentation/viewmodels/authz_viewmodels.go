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

// AuthzRequestDetail represents the data required to render the request detail page.
type AuthzRequestDetail struct {
	ID          string
	Status      string
	StatusClass string
	Requester   string
	CreatedAt   string
	UpdatedAt   string
	ReviewedAt  string
	Object      string
	Action      string
	Domain      string
	Reason      string
	DiffJSON    string
	DiffKind    string
	Diff        []PolicyDiffItem
	Suggestions []AuthzPolicySuggestionItem
	BotLog      string
	PRLink      string
	CanReview   bool
	CanCancel   bool
	CanRevert   bool
	CanRetryBot bool
	RetryToken  string
}

type AuthzRequestListItem struct {
	ID          string
	Status      string
	StatusClass string
	Object      string
	Action      string
	Domain      string
	Reason      string
	CreatedAt   string
	UpdatedAt   string
	ViewURL     string
}

type AuthzRequestList struct {
	Items     []AuthzRequestListItem
	Total     int64
	Page      int
	Limit     int
	Mine      bool
	Statuses  []string
	Subject   string
	Domain    string
	CanReview bool
}
