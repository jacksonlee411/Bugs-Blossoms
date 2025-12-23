package viewmodels

type PersonListItem struct {
	PersonUUID  string
	Pernr       string
	DisplayName string
	Status      string
}

type PersonsListPageProps struct {
	Items      []*PersonListItem
	Q          string
	NewURL     string
	CanCreate  bool
	CanRequest bool
	CanDebug   bool
}

type PersonDetailPageProps struct {
	Person        *PersonListItem
	BackURL       string
	CanRequest    bool
	CanDebug      bool
	EffectiveDate string
	Step          string
}

type PersonCreatePageProps struct {
	Errors map[string]string
	Form   *PersonCreateFormVM
	PostTo string
}

type PersonCreateFormVM struct {
	Pernr       string
	DisplayName string
}
