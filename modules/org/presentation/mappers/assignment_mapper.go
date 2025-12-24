package mappers

import (
	"strings"

	"github.com/iota-uz/iota-sdk/modules/org/presentation/viewmodels"
	"github.com/iota-uz/iota-sdk/modules/org/services"
)

func AssignmentsToTimeline(subject string, rows []services.AssignmentViewRow) *viewmodels.OrgAssignmentsTimeline {
	pernr := ""
	if strings.HasPrefix(subject, "person:") {
		pernr = strings.TrimPrefix(subject, "person:")
	}
	out := make([]viewmodels.OrgAssignmentRow, 0, len(rows))
	for _, r := range rows {
		code := ""
		if r.PositionCode != nil {
			code = *r.PositionCode
		}
		startEventType := ""
		if r.StartEventType != nil {
			startEventType = strings.TrimSpace(*r.StartEventType)
		}
		endEventType := ""
		if r.EndEventType != nil {
			endEventType = strings.TrimSpace(*r.EndEventType)
		}
		p := pernr
		if r.Pernr != nil && strings.TrimSpace(*r.Pernr) != "" {
			p = strings.TrimSpace(*r.Pernr)
		}
		out = append(out, viewmodels.OrgAssignmentRow{
			ID:            r.ID,
			PositionID:    r.PositionID,
			OrgNodeID:     r.OrgNodeID,
			Pernr:         p,
			PositionCode:  code,
			PositionLabel: strings.TrimSpace(code),
			OperationType: startEventType,
			EndEventType:  endEventType,
			EffectiveDate: r.EffectiveDate,
			EndDate:       r.EndDate,
		})
	}
	return &viewmodels.OrgAssignmentsTimeline{
		Subject: subject,
		Pernr:   pernr,
		Rows:    out,
	}
}
