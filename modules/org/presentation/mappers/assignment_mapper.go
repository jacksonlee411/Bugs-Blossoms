package mappers

import (
	"fmt"
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
		title := ""
		if r.PositionTitle != nil {
			title = *r.PositionTitle
		}
		orgNodeCode := ""
		if r.OrgNodeCode != nil {
			orgNodeCode = *r.OrgNodeCode
		}
		orgNodeName := ""
		if r.OrgNodeName != nil {
			orgNodeName = *r.OrgNodeName
		}

		orgLabel := strings.TrimSpace(r.OrgNodeID.String())
		orgNodeName = strings.TrimSpace(orgNodeName)
		orgNodeCode = strings.TrimSpace(orgNodeCode)
		if orgNodeName != "" && orgNodeCode != "" {
			orgLabel = fmt.Sprintf("%s (%s)", orgNodeName, orgNodeCode)
		} else if orgNodeName != "" {
			orgLabel = orgNodeName
		} else if orgNodeCode != "" {
			orgLabel = orgNodeCode
		}

		posLabel := strings.TrimSpace(code)
		title = strings.TrimSpace(title)
		if title != "" {
			if posLabel != "" {
				posLabel = fmt.Sprintf("%s — %s", posLabel, title)
			} else {
				posLabel = title
			}
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
			OrgNodeLabel:  strings.TrimSpace(orgLabel),
			PositionLabel: strings.TrimSpace(posLabel),
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
