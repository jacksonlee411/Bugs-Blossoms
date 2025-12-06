package viewmodels

type AuthenticationLog struct {
	ID        uint
	UserID    uint
	IP        string
	UserAgent string
	CreatedAt string
}

type ActionLog struct {
	ID        uint
	UserID    string
	Method    string
	Path      string
	IP        string
	UserAgent string
	CreatedAt string
}

type AuthenticationFilters struct {
	UserID    string
	IP        string
	UserAgent string
	From      string
	To        string
}

type ActionFilters struct {
	UserID    string
	Method    string
	Path      string
	IP        string
	UserAgent string
	From      string
	To        string
}

type AuthenticationSection struct {
	Logs    []*AuthenticationLog
	Total   int64
	Filters AuthenticationFilters
	Page    int
	PerPage int
}

type ActionSection struct {
	Logs    []*ActionLog
	Total   int64
	Filters ActionFilters
	Page    int
	PerPage int
}

type LogsPageProps struct {
	BasePath       string
	ActiveTab      string
	Authentication AuthenticationSection
	Action         ActionSection
}
