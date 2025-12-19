package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	qualityRulesetName    = "org-quality"
	qualityRulesetVersion = "v1"

	qualityReportSchemaVersion   = 1
	qualityFixPlanSchemaVersion  = 1
	qualityFixManSchemaVersion   = 1
	qualityMaxIssuesDefaultLimit = 10000
)

const (
	ruleNodeCodeFormat           = "ORG_Q_001_NODE_CODE_FORMAT"
	rulePositionCodeFormat       = "ORG_Q_002_POSITION_CODE_FORMAT"
	ruleRootInvariants           = "ORG_Q_003_ROOT_INVARIANTS"
	ruleNodeMissingSliceAsOf     = "ORG_Q_004_NODE_MISSING_SLICE_ASOF"
	ruleNodeMissingEdgeAsOf      = "ORG_Q_005_NODE_MISSING_EDGE_ASOF"
	ruleEdgeParentNullNonRoot    = "ORG_Q_006_EDGE_PARENT_NULL_FOR_NON_ROOT"
	ruleLeafRequiresPositionAsOf = "ORG_Q_007_LEAF_REQUIRES_POSITION_ASOF"
	ruleAssignmentSubjectMapping = "ORG_Q_008_ASSIGNMENT_SUBJECT_MAPPING"
)

const (
	severityError   = "error"
	severityWarning = "warning"
)

const (
	fixKindAssignmentCorrect = "assignment.correct"
)

var (
	nodeCodeRegex     = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,63}$`)
	positionCodeRegex = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,63}$`)
	autoPositionRegex = regexp.MustCompile(`^AUTO-[0-9A-F]{16}$`)
)

type qualityRuleset struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type qualitySummary struct {
	Errors      int  `json:"errors"`
	Warnings    int  `json:"warnings"`
	IssuesTotal int  `json:"issues_total"`
	Truncated   bool `json:"truncated,omitempty"`
}

type qualityEntityRef struct {
	Type string    `json:"type"`
	ID   uuid.UUID `json:"id"`
}

type qualityEffectiveWindow struct {
	EffectiveDate time.Time `json:"effective_date"`
	EndDate       time.Time `json:"end_date"`
}

type qualityAutofix struct {
	Supported bool   `json:"supported"`
	FixKind   string `json:"fix_kind"`
	Risk      string `json:"risk"`
}

type qualityIssue struct {
	IssueID         uuid.UUID               `json:"issue_id"`
	RuleID          string                  `json:"rule_id"`
	Severity        string                  `json:"severity"`
	Entity          qualityEntityRef        `json:"entity"`
	EffectiveWindow *qualityEffectiveWindow `json:"effective_window,omitempty"`
	Message         string                  `json:"message"`
	Details         map[string]any          `json:"details,omitempty"`
	Autofix         *qualityAutofix         `json:"autofix,omitempty"`
}

type qualityReportV1 struct {
	SchemaVersion int            `json:"schema_version"`
	RunID         uuid.UUID      `json:"run_id"`
	TenantID      uuid.UUID      `json:"tenant_id"`
	AsOf          time.Time      `json:"as_of"`
	GeneratedAt   time.Time      `json:"generated_at"`
	Ruleset       qualityRuleset `json:"ruleset"`
	Summary       qualitySummary `json:"summary"`
	Issues        []qualityIssue `json:"issues"`
}

type qualityBatchCommand struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type qualityBatchRequest struct {
	DryRun        bool                  `json:"dry_run"`
	EffectiveDate string                `json:"effective_date"`
	Commands      []qualityBatchCommand `json:"commands"`
}

type qualityFixPlanMaps struct {
	IssueToCommandIndexes map[string][]int `json:"issue_to_command_indexes"`
}

type qualityFixPlanV1 struct {
	SchemaVersion     int                 `json:"schema_version"`
	RunID             uuid.UUID           `json:"run_id"`
	TenantID          uuid.UUID           `json:"tenant_id"`
	AsOf              time.Time           `json:"as_of"`
	SourceReportRunID uuid.UUID           `json:"source_report_run_id"`
	CreatedAt         time.Time           `json:"created_at"`
	BatchRequest      qualityBatchRequest `json:"batch_request"`
	Maps              qualityFixPlanMaps  `json:"maps"`
}

type qualityBeforeAssignment struct {
	ID         uuid.UUID `json:"id"`
	Pernr      string    `json:"pernr"`
	SubjectID  uuid.UUID `json:"subject_id"`
	PositionID uuid.UUID `json:"position_id"`
}

type qualityFixBefore struct {
	Assignments []qualityBeforeAssignment `json:"assignments"`
}

type qualityBatchCommandResult struct {
	Index  int            `json:"index"`
	Type   string         `json:"type"`
	Ok     bool           `json:"ok"`
	Result map[string]any `json:"result,omitempty"`
}

type qualityFixResults struct {
	Ok             bool                        `json:"ok"`
	EventsEnqueued int                         `json:"events_enqueued"`
	BatchResults   []qualityBatchCommandResult `json:"batch_results"`
	Error          *apiError                   `json:"error,omitempty"`
}

type qualityFixManifestV1 struct {
	SchemaVersion        int                 `json:"schema_version"`
	RunID                uuid.UUID           `json:"run_id"`
	TenantID             uuid.UUID           `json:"tenant_id"`
	AsOf                 time.Time           `json:"as_of"`
	AppliedAt            time.Time           `json:"applied_at"`
	SourceFixPlanRunID   uuid.UUID           `json:"source_fix_plan_run_id"`
	ChangeRequestID      *uuid.UUID          `json:"change_request_id"`
	BatchRequest         qualityBatchRequest `json:"batch_request"`
	Before               qualityFixBefore    `json:"before"`
	Results              qualityFixResults   `json:"results"`
	PreflightResponseRaw json.RawMessage     `json:"preflight_response,omitempty"`
}

func (r *qualityReportV1) validate() error {
	if r == nil {
		return withCode(exitValidation, fmt.Errorf("report is nil"))
	}
	if r.SchemaVersion != qualityReportSchemaVersion {
		return withCode(exitValidation, fmt.Errorf("report schema_version=%d is unsupported", r.SchemaVersion))
	}
	if r.RunID == uuid.Nil {
		return withCode(exitValidation, fmt.Errorf("report.run_id is required"))
	}
	if r.TenantID == uuid.Nil {
		return withCode(exitValidation, fmt.Errorf("report.tenant_id is required"))
	}
	if r.AsOf.IsZero() {
		return withCode(exitValidation, fmt.Errorf("report.as_of is required"))
	}
	if r.Ruleset.Name != qualityRulesetName || r.Ruleset.Version != qualityRulesetVersion {
		return withCode(exitValidation, fmt.Errorf("report.ruleset must be %s/%s", qualityRulesetName, qualityRulesetVersion))
	}
	for i, iss := range r.Issues {
		if iss.IssueID == uuid.Nil {
			return withCode(exitValidation, fmt.Errorf("report.issues[%d].issue_id is required", i))
		}
		if strings.TrimSpace(iss.RuleID) == "" {
			return withCode(exitValidation, fmt.Errorf("report.issues[%d].rule_id is required", i))
		}
		if iss.Severity != severityError && iss.Severity != severityWarning {
			return withCode(exitValidation, fmt.Errorf("report.issues[%d].severity is invalid", i))
		}
		if strings.TrimSpace(iss.Entity.Type) == "" || iss.Entity.ID == uuid.Nil {
			return withCode(exitValidation, fmt.Errorf("report.issues[%d].entity is required", i))
		}
		if strings.TrimSpace(iss.Message) == "" {
			return withCode(exitValidation, fmt.Errorf("report.issues[%d].message is required", i))
		}
		if iss.Autofix != nil && iss.Autofix.Supported {
			if strings.TrimSpace(iss.Autofix.FixKind) == "" {
				return withCode(exitValidation, fmt.Errorf("report.issues[%d].autofix.fix_kind is required", i))
			}
		}
	}
	return nil
}

func (p *qualityFixPlanV1) validate() error {
	if p == nil {
		return withCode(exitValidation, fmt.Errorf("fix plan is nil"))
	}
	if p.SchemaVersion != qualityFixPlanSchemaVersion {
		return withCode(exitValidation, fmt.Errorf("fix_plan schema_version=%d is unsupported", p.SchemaVersion))
	}
	if p.RunID == uuid.Nil {
		return withCode(exitValidation, fmt.Errorf("fix_plan.run_id is required"))
	}
	if p.TenantID == uuid.Nil {
		return withCode(exitValidation, fmt.Errorf("fix_plan.tenant_id is required"))
	}
	if p.AsOf.IsZero() {
		return withCode(exitValidation, fmt.Errorf("fix_plan.as_of is required"))
	}
	if p.SourceReportRunID == uuid.Nil {
		return withCode(exitValidation, fmt.Errorf("fix_plan.source_report_run_id is required"))
	}
	if strings.TrimSpace(p.BatchRequest.EffectiveDate) == "" {
		return withCode(exitValidation, fmt.Errorf("fix_plan.batch_request.effective_date is required"))
	}
	if len(p.BatchRequest.Commands) == 0 {
		return withCode(exitValidation, fmt.Errorf("fix_plan.batch_request.commands is required"))
	}
	for i, cmd := range p.BatchRequest.Commands {
		if strings.TrimSpace(cmd.Type) != fixKindAssignmentCorrect {
			return withCode(exitValidation, fmt.Errorf("fix_plan.batch_request.commands[%d].type=%q is not allowed in v1", i, cmd.Type))
		}
		if len(cmd.Payload) == 0 {
			return withCode(exitValidation, fmt.Errorf("fix_plan.batch_request.commands[%d].payload is required", i))
		}
	}
	return nil
}

func (m *qualityFixManifestV1) validate() error {
	if m == nil {
		return withCode(exitValidation, fmt.Errorf("manifest is nil"))
	}
	if m.SchemaVersion != qualityFixManSchemaVersion {
		return withCode(exitValidation, fmt.Errorf("manifest schema_version=%d is unsupported", m.SchemaVersion))
	}
	if m.RunID == uuid.Nil {
		return withCode(exitValidation, fmt.Errorf("manifest.run_id is required"))
	}
	if m.TenantID == uuid.Nil {
		return withCode(exitValidation, fmt.Errorf("manifest.tenant_id is required"))
	}
	if m.AsOf.IsZero() {
		return withCode(exitValidation, fmt.Errorf("manifest.as_of is required"))
	}
	if m.SourceFixPlanRunID == uuid.Nil {
		return withCode(exitValidation, fmt.Errorf("manifest.source_fix_plan_run_id is required"))
	}
	if strings.TrimSpace(m.BatchRequest.EffectiveDate) == "" {
		return withCode(exitValidation, fmt.Errorf("manifest.batch_request.effective_date is required"))
	}
	if len(m.BatchRequest.Commands) == 0 {
		return withCode(exitValidation, fmt.Errorf("manifest.batch_request.commands is required"))
	}
	return nil
}

func qualityAsOfDateString(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

func qualityFileAsOfToken(t time.Time) string {
	return t.UTC().Format("20060102")
}

func qualityReportFilePath(outputDir string, tenantID uuid.UUID, asOf time.Time, runID uuid.UUID) string {
	name := fmt.Sprintf("org_quality_report_%s_%s_%s.json", tenantID.String(), qualityFileAsOfToken(asOf), runID.String())
	return filepath.Join(outputDir, name)
}

func qualityFixPlanFilePath(outputDir string, tenantID uuid.UUID, asOf time.Time, runID uuid.UUID) string {
	name := fmt.Sprintf("org_quality_fix_plan_%s_%s_%s.json", tenantID.String(), qualityFileAsOfToken(asOf), runID.String())
	return filepath.Join(outputDir, name)
}

func qualityFixManifestFilePath(outputDir string, tenantID uuid.UUID, asOf time.Time, runID uuid.UUID) string {
	name := fmt.Sprintf("org_quality_fix_manifest_%s_%s_%s.json", tenantID.String(), qualityFileAsOfToken(asOf), runID.String())
	return filepath.Join(outputDir, name)
}

func sortQualityIssues(issues []qualityIssue) {
	sort.SliceStable(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		if a.Severity != b.Severity {
			return a.Severity < b.Severity
		}
		if a.Entity.Type != b.Entity.Type {
			return a.Entity.Type < b.Entity.Type
		}
		return a.Entity.ID.String() < b.Entity.ID.String()
	})
}
