package mappers

import (
	"github.com/iota-uz/iota-sdk/modules/person/domain/aggregates/person"
	"github.com/iota-uz/iota-sdk/modules/person/presentation/viewmodels"
)

func PersonToListItem(p person.Person) *viewmodels.PersonListItem {
	return &viewmodels.PersonListItem{
		PersonUUID:  p.PersonUUID().String(),
		Pernr:       p.Pernr(),
		DisplayName: p.DisplayName(),
		Status:      string(p.Status()),
	}
}
