package mappers

import (
	"encoding/json"

	"github.com/iota-uz/iota-sdk/modules/org/presentation/viewmodels"
	"github.com/iota-uz/iota-sdk/modules/org/services"
)

func NodeDetailsToViewModel(n *services.NodeAsOf) *viewmodels.OrgNodeDetails {
	if n == nil {
		return nil
	}
	i18n := ""
	if n.Slice.I18nNames != nil {
		if b, err := json.MarshalIndent(n.Slice.I18nNames, "", "  "); err == nil {
			i18n = string(b)
		}
	}
	return &viewmodels.OrgNodeDetails{
		ID:            n.Node.ID,
		Code:          n.Node.Code,
		Name:          n.Slice.Name,
		Status:        n.Slice.Status,
		DisplayOrder:  n.Slice.DisplayOrder,
		ParentHint:    n.Slice.ParentHint,
		LegalEntityID: n.Slice.LegalEntityID,
		CompanyCode:   n.Slice.CompanyCode,
		LocationID:    n.Slice.LocationID,
		ManagerUserID: n.Slice.ManagerUserID,
		EffectiveDate: n.Slice.EffectiveDate,
		EndDate:       n.Slice.EndDate,
		I18nNamesJSON: i18n,
		IsRoot:        n.Node.IsRoot,
	}
}
