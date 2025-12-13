package viewmodels

// AuthzChangesSummary represents the summary of staged changes in the workspace.
type AuthzChangesSummary struct {
	Added     int
	Removed   int
	Resources []string
}

// PolicyDiffItem describes a single diff operation for display.
type PolicyDiffItem struct {
	Op   string
	Path string
	Text string
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
	Diff        []PolicyDiffItem
	BotLog      string
	PRLink      string
}
