package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/iota-uz/iota-sdk/pkg/composables"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
)

type importOptions struct {
	tenantID        uuid.UUID
	inputDir        string
	outputDir       string
	apply           bool
	skipAssignments bool
	strict          bool
	backend         string
	mode            string
}

func pgDateOnlyUTC(t time.Time) pgtype.Date {
	u := dateOnlyUTC(t)
	if u.IsZero() {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: u, Valid: true}
}

func dateOnlyUTC(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	u := t.UTC()
	y, m, d := u.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func newImportCmd() *cobra.Command {
	var opts importOptions

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import Org data from CSV directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			return runImport(ctx, opts)
		},
	}

	cmd.Flags().StringVar(&opts.inputDir, "input", "", "Input directory containing CSV files (required)")
	cmd.Flags().StringVar(&opts.outputDir, "output", "", "Output directory for manifest (default: input dir)")
	cmd.Flags().BoolVar(&opts.apply, "apply", false, "Apply changes to DB (default is dry-run)")
	cmd.Flags().BoolVar(&opts.skipAssignments, "skip-assignments", false, "Skip assignments import even if assignments.csv exists")
	cmd.Flags().BoolVar(&opts.strict, "strict", false, "Strict cycle checks across all effective dates")
	cmd.Flags().StringVar(&opts.backend, "backend", "db", "Backend: db (MVP)")
	cmd.Flags().StringVar(&opts.mode, "mode", "seed", "Mode: seed (MVP)")

	var tenant string
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant UUID (required)")

	_ = cmd.MarkFlagRequired("tenant")
	_ = cmd.MarkFlagRequired("input")

	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		id, err := uuid.Parse(strings.TrimSpace(tenant))
		if err != nil {
			return withCode(exitUsage, fmt.Errorf("invalid --tenant: %w", err))
		}
		opts.tenantID = id
		return nil
	}

	return cmd
}

type nodeCSVRow struct {
	line int

	code          string
	nodeType      string
	name          string
	i18nNames     json.RawMessage
	status        string
	legalEntityID *uuid.UUID
	companyCode   *string
	locationID    *uuid.UUID
	displayOrder  int
	parentCode    *string
	managerUserID *int64
	managerEmail  *string

	effectiveDate   time.Time
	endDate         time.Time
	endDateProvided bool
}

type positionCSVRow struct {
	line int

	code            string
	orgNodeCode     string
	title           *string
	status          string
	isAutoCreated   bool
	effectiveDate   time.Time
	endDate         time.Time
	endDateProvided bool
}

type assignmentCSVRow struct {
	line int

	positionCode   string
	assignmentType string
	pernr          string
	subjectID      *uuid.UUID

	effectiveDate   time.Time
	endDate         time.Time
	endDateProvided bool
}

type normalizedData struct {
	runID     uuid.UUID
	tenantID  uuid.UUID
	startedAt time.Time

	nodesByCode     map[string]uuid.UUID
	nodeSlices      []nodeCSVRow
	edges           []edgeRow
	positions       []positionRow
	assignments     []assignmentRow
	subjectMappings map[string]uuid.UUID // pernr -> subject_id
}

type edgeRow struct {
	id            uuid.UUID
	childCode     string
	parentCode    *string
	effectiveDate time.Time
	endDate       time.Time
}

type positionRow struct {
	id            uuid.UUID
	line          int
	code          string
	orgNodeCode   string
	orgNodeID     uuid.UUID
	title         *string
	status        string
	isAutoCreated bool
	effectiveDate time.Time
	endDate       time.Time
}

type assignmentRow struct {
	id             uuid.UUID
	line           int
	positionCode   string
	positionID     uuid.UUID
	assignmentType string
	pernr          string
	subjectID      uuid.UUID
	effectiveDate  time.Time
	endDate        time.Time
}

func runImport(ctx context.Context, opts importOptions) error {
	if strings.TrimSpace(opts.inputDir) == "" {
		return withCode(exitUsage, fmt.Errorf("--input is required"))
	}
	if opts.outputDir == "" {
		opts.outputDir = opts.inputDir
	}
	if opts.backend != "db" {
		return withCode(exitUsage, fmt.Errorf("unsupported --backend: %s", opts.backend))
	}
	if opts.mode != "seed" {
		return withCode(exitUsage, fmt.Errorf("unsupported --mode for backend=db: %s", opts.mode))
	}
	if opts.tenantID == uuid.Nil {
		return withCode(exitUsage, fmt.Errorf("--tenant is required"))
	}

	startedAt := time.Now().UTC()
	runID := uuid.New()

	nodesPath := filepath.Join(opts.inputDir, "nodes.csv")
	nodes, err := parseNodesCSV(nodesPath)
	if err != nil {
		return withCode(exitValidation, fmt.Errorf("nodes.csv: %w", err))
	}

	positionsPath := filepath.Join(opts.inputDir, "positions.csv")
	positionsRaw, err := parsePositionsCSVIfExists(positionsPath)
	if err != nil {
		return withCode(exitValidation, fmt.Errorf("positions.csv: %w", err))
	}

	var assignmentsRaw []assignmentCSVRow
	if !opts.skipAssignments {
		assignmentsPath := filepath.Join(opts.inputDir, "assignments.csv")
		parsedAssignments, err := parseAssignmentsCSVIfExists(assignmentsPath)
		if err != nil {
			return withCode(exitValidation, fmt.Errorf("assignments.csv: %w", err))
		}
		assignmentsRaw = parsedAssignments
	}

	data, err := normalizeAndValidate(runID, opts.tenantID, startedAt, nodes, positionsRaw, assignmentsRaw, opts.strict)
	if err != nil {
		return withCode(exitValidation, err)
	}

	pool, err := connectDB(ctx)
	if err != nil {
		return withCode(exitDB, err)
	}
	defer pool.Close()

	if err := dbPrecheckSeed(ctx, pool, data.tenantID); err != nil {
		return err
	}
	if err := resolveManagers(ctx, pool, &data); err != nil {
		return err
	}
	if !opts.skipAssignments {
		if err := resolvePersons(ctx, pool, &data); err != nil {
			return err
		}
	}

	if !opts.apply {
		return printImportSummary(data, opts, "dry_run", nil)
	}

	manifest, err := applySeedImport(ctx, pool, data, opts)
	if err != nil {
		return err
	}

	if err := writeManifest(opts.outputDir, manifest); err != nil {
		return withCode(exitDB, fmt.Errorf("write manifest: %w", err))
	}
	return printImportSummary(data, opts, "applied", manifest)
}

func parseNodesCSV(path string) ([]nodeCSVRow, error) {
	r, closeFn, err := openCSV(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = closeFn() }()

	header, err := readHeader(r)
	if err != nil {
		return nil, err
	}
	required := []string{"code", "name", "effective_date"}
	allowed := []string{
		"code", "type", "name", "i18n_names", "status", "legal_entity_id", "company_code", "location_id",
		"display_order", "parent_code", "manager_user_id", "manager_email", "effective_date", "end_date",
	}
	if err := requireHeader(header, required, allowed); err != nil {
		return nil, err
	}
	idx := headerIndex(header)

	var rows []nodeCSVRow
	line := 1
	for {
		line++
		rec, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if len(rec) == 0 {
			continue
		}

		get := func(name string) string {
			i, ok := idx[name]
			if !ok || i >= len(rec) {
				return ""
			}
			return rec[i]
		}

		code := strings.TrimSpace(get("code"))
		if code == "" {
			return nil, fmt.Errorf("line %d: code is required", line)
		}

		name := strings.TrimSpace(get("name"))
		if name == "" {
			return nil, fmt.Errorf("line %d: name is required", line)
		}

		nodeType := strings.TrimSpace(get("type"))
		if nodeType == "" {
			nodeType = "OrgUnit"
		}
		status := strings.TrimSpace(get("status"))
		if status == "" {
			status = "active"
		}

		i18nNames, err := parseJSONObjectField(get("i18n_names"))
		if err != nil {
			return nil, fmt.Errorf("line %d: i18n_names: %w", line, err)
		}

		var legalEntityID *uuid.UUID
		if v := strings.TrimSpace(get("legal_entity_id")); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				return nil, fmt.Errorf("line %d: legal_entity_id: %w", line, err)
			}
			legalEntityID = &id
		}
		var locationID *uuid.UUID
		if v := strings.TrimSpace(get("location_id")); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				return nil, fmt.Errorf("line %d: location_id: %w", line, err)
			}
			locationID = &id
		}

		var companyCode *string
		if v := strings.TrimSpace(get("company_code")); v != "" {
			companyCode = &v
		}

		displayOrder := 0
		if v := strings.TrimSpace(get("display_order")); v != "" {
			i, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("line %d: display_order: %w", line, err)
			}
			displayOrder = i
		}

		var parentCode *string
		if v := strings.TrimSpace(get("parent_code")); v != "" {
			parentCode = &v
		}

		var managerUserID *int64
		if v := strings.TrimSpace(get("manager_user_id")); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("line %d: manager_user_id: %w", line, err)
			}
			managerUserID = &n
		}
		var managerEmail *string
		if v := strings.TrimSpace(get("manager_email")); v != "" {
			managerEmail = &v
		}

		effectiveDate, err := parseTimeField(get("effective_date"))
		if err != nil {
			return nil, fmt.Errorf("line %d: effective_date: %w", line, err)
		}
		effectiveDate = dateOnlyUTC(effectiveDate)

		var endDate time.Time
		endProvided := false
		if v := strings.TrimSpace(get("end_date")); v != "" {
			t, err := parseTimeField(v)
			if err != nil {
				return nil, fmt.Errorf("line %d: end_date: %w", line, err)
			}
			endDate = dateOnlyUTC(t)
			endProvided = true
		}

		rows = append(rows, nodeCSVRow{
			line:            line,
			code:            code,
			nodeType:        nodeType,
			name:            name,
			i18nNames:       i18nNames,
			status:          status,
			legalEntityID:   legalEntityID,
			companyCode:     companyCode,
			locationID:      locationID,
			displayOrder:    displayOrder,
			parentCode:      parentCode,
			managerUserID:   managerUserID,
			managerEmail:    managerEmail,
			effectiveDate:   effectiveDate,
			endDate:         endDate,
			endDateProvided: endProvided,
		})
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("no data rows found")
	}
	return rows, nil
}

func parsePositionsCSVIfExists(path string) ([]positionCSVRow, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	r, closeFn, err := openCSV(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = closeFn() }()

	header, err := readHeader(r)
	if err != nil {
		return nil, err
	}
	required := []string{"code", "org_node_code", "effective_date"}
	allowed := []string{"code", "org_node_code", "title", "status", "is_auto_created", "effective_date", "end_date"}
	if err := requireHeader(header, required, allowed); err != nil {
		return nil, err
	}
	idx := headerIndex(header)

	var rows []positionCSVRow
	line := 1
	for {
		line++
		rec, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		get := func(name string) string {
			i, ok := idx[name]
			if !ok || i >= len(rec) {
				return ""
			}
			return rec[i]
		}

		code := strings.TrimSpace(get("code"))
		if code == "" {
			return nil, fmt.Errorf("line %d: code is required", line)
		}
		orgNodeCode := strings.TrimSpace(get("org_node_code"))
		if orgNodeCode == "" {
			return nil, fmt.Errorf("line %d: org_node_code is required", line)
		}

		var title *string
		if v := strings.TrimSpace(get("title")); v != "" {
			title = &v
		}
		status := strings.TrimSpace(get("status"))
		if status == "" {
			status = "active"
		}

		isAuto := false
		if v := strings.TrimSpace(get("is_auto_created")); v != "" {
			b, err := strconv.ParseBool(v)
			if err != nil {
				return nil, fmt.Errorf("line %d: is_auto_created: %w", line, err)
			}
			isAuto = b
		}

		effectiveDate, err := parseTimeField(get("effective_date"))
		if err != nil {
			return nil, fmt.Errorf("line %d: effective_date: %w", line, err)
		}
		effectiveDate = dateOnlyUTC(effectiveDate)

		var endDate time.Time
		endProvided := false
		if v := strings.TrimSpace(get("end_date")); v != "" {
			t, err := parseTimeField(v)
			if err != nil {
				return nil, fmt.Errorf("line %d: end_date: %w", line, err)
			}
			endDate = dateOnlyUTC(t)
			endProvided = true
		}

		rows = append(rows, positionCSVRow{
			line:            line,
			code:            code,
			orgNodeCode:     orgNodeCode,
			title:           title,
			status:          status,
			isAutoCreated:   isAuto,
			effectiveDate:   effectiveDate,
			endDate:         endDate,
			endDateProvided: endProvided,
		})
	}

	return rows, nil
}

func parseAssignmentsCSVIfExists(path string) ([]assignmentCSVRow, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	r, closeFn, err := openCSV(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = closeFn() }()

	header, err := readHeader(r)
	if err != nil {
		return nil, err
	}
	required := []string{"position_code", "pernr", "effective_date"}
	allowed := []string{"position_code", "assignment_type", "pernr", "subject_id", "effective_date", "end_date"}
	if err := requireHeader(header, required, allowed); err != nil {
		return nil, err
	}
	idx := headerIndex(header)

	var rows []assignmentCSVRow
	line := 1
	for {
		line++
		rec, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("line %d: %w", line, err)
		}

		get := func(name string) string {
			i, ok := idx[name]
			if !ok || i >= len(rec) {
				return ""
			}
			return rec[i]
		}

		positionCode := strings.TrimSpace(get("position_code"))
		if positionCode == "" {
			return nil, fmt.Errorf("line %d: position_code is required", line)
		}

		assignmentType := strings.TrimSpace(get("assignment_type"))
		if assignmentType == "" {
			assignmentType = "primary"
		}

		pernr := strings.TrimSpace(get("pernr"))
		if pernr == "" {
			return nil, fmt.Errorf("line %d: pernr is required", line)
		}

		var subjectID *uuid.UUID
		if v := strings.TrimSpace(get("subject_id")); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				return nil, fmt.Errorf("line %d: subject_id: %w", line, err)
			}
			subjectID = &id
		}

		effectiveDate, err := parseTimeField(get("effective_date"))
		if err != nil {
			return nil, fmt.Errorf("line %d: effective_date: %w", line, err)
		}
		effectiveDate = dateOnlyUTC(effectiveDate)

		var endDate time.Time
		endProvided := false
		if v := strings.TrimSpace(get("end_date")); v != "" {
			t, err := parseTimeField(v)
			if err != nil {
				return nil, fmt.Errorf("line %d: end_date: %w", line, err)
			}
			endDate = dateOnlyUTC(t)
			endProvided = true
		}

		rows = append(rows, assignmentCSVRow{
			line:            line,
			positionCode:    positionCode,
			assignmentType:  assignmentType,
			pernr:           pernr,
			subjectID:       subjectID,
			effectiveDate:   effectiveDate,
			endDate:         endDate,
			endDateProvided: endProvided,
		})
	}

	return rows, nil
}

func normalizeAndValidate(
	runID uuid.UUID,
	tenantID uuid.UUID,
	startedAt time.Time,
	nodes []nodeCSVRow,
	positions []positionCSVRow,
	assignments []assignmentCSVRow,
	strict bool,
) (normalizedData, error) {
	if tenantID == uuid.Nil {
		return normalizedData{}, fmt.Errorf("tenant_id is required")
	}

	nodesByCode := make(map[string]uuid.UUID)
	for _, r := range nodes {
		if _, ok := nodesByCode[r.code]; !ok {
			nodesByCode[r.code] = uuid.New()
		}
	}

	rootCode, err := validateAndNormalizeNodes(&nodes, nodesByCode)
	if err != nil {
		return normalizedData{}, err
	}

	edges, err := deriveEdges(nodes)
	if err != nil {
		return normalizedData{}, err
	}

	if err := validateParentReferences(nodes, rootCode); err != nil {
		return normalizedData{}, err
	}
	if err := validateCycles(nodes, edges, strict); err != nil {
		return normalizedData{}, err
	}

	posRows, err := normalizePositions(positions, nodesByCode)
	if err != nil {
		return normalizedData{}, err
	}

	assignRows, mappings, err := normalizeAssignments(tenantID, assignments, posRows)
	if err != nil {
		return normalizedData{}, err
	}

	return normalizedData{
		runID:           runID,
		tenantID:        tenantID,
		startedAt:       startedAt,
		nodesByCode:     nodesByCode,
		nodeSlices:      nodes,
		edges:           edges,
		positions:       posRows,
		assignments:     assignRows,
		subjectMappings: mappings,
	}, nil
}

func validateAndNormalizeNodes(rows *[]nodeCSVRow, nodesByCode map[string]uuid.UUID) (string, error) {
	var rootCode string
	for _, r := range *rows {
		if r.nodeType != "OrgUnit" {
			return "", fmt.Errorf("line %d: type must be OrgUnit, got %q", r.line, r.nodeType)
		}
		switch r.status {
		case "active", "retired", "rescinded":
		default:
			return "", fmt.Errorf("line %d: invalid status: %q", r.line, r.status)
		}

		if r.parentCode == nil || strings.TrimSpace(*r.parentCode) == "" {
			if rootCode == "" {
				rootCode = r.code
			} else if rootCode != r.code {
				return "", fmt.Errorf("line %d: multiple root codes found: %q and %q", r.line, rootCode, r.code)
			}
		}
	}
	if rootCode == "" {
		return "", fmt.Errorf("root code is required (parent_code empty)")
	}

	group := make(map[string][]int)
	for i := range *rows {
		group[(*rows)[i].code] = append(group[(*rows)[i].code], i)
	}
	for code, idxs := range group {
		sort.Slice(idxs, func(i, j int) bool {
			return (*rows)[idxs[i]].effectiveDate.Before((*rows)[idxs[j]].effectiveDate)
		})
		for j := range idxs {
			i := idxs[j]
			if !(*rows)[i].endDateProvided {
				if j+1 < len(idxs) {
					(*rows)[i].endDate = (*rows)[idxs[j+1]].effectiveDate.AddDate(0, 0, -1)
				} else {
					(*rows)[i].endDate = maxEndDate
				}
			}
			if (*rows)[i].effectiveDate.After((*rows)[i].endDate) {
				return "", fmt.Errorf("line %d: effective_date must be <= end_date", (*rows)[i].line)
			}
			if j+1 < len(idxs) {
				next := (*rows)[idxs[j+1]]
				if !(*rows)[i].endDate.Before(next.effectiveDate) {
					return "", fmt.Errorf("line %d: overlapping time windows for code=%s", (*rows)[i].line, code)
				}
			}
		}
	}

	// root can't move: all slices must keep parent_code empty.
	for _, r := range *rows {
		if r.code != rootCode {
			continue
		}
		if r.parentCode != nil && strings.TrimSpace(*r.parentCode) != "" {
			return "", fmt.Errorf("line %d: root code %q must have empty parent_code", r.line, rootCode)
		}
	}

	// validate referenced node codes exist
	for _, r := range *rows {
		if r.parentCode == nil {
			continue
		}
		p := strings.TrimSpace(*r.parentCode)
		if p == "" {
			continue
		}
		if _, ok := nodesByCode[p]; !ok {
			return "", fmt.Errorf("line %d: unknown parent_code: %q", r.line, p)
		}
	}

	return rootCode, nil
}

func deriveEdges(nodes []nodeCSVRow) ([]edgeRow, error) {
	edges := make([]edgeRow, 0, len(nodes))
	for _, r := range nodes {
		var parent *string
		if r.parentCode != nil {
			v := strings.TrimSpace(*r.parentCode)
			if v != "" {
				parent = &v
			}
		}
		edges = append(edges, edgeRow{
			id:            uuid.New(),
			childCode:     r.code,
			parentCode:    parent,
			effectiveDate: r.effectiveDate,
			endDate:       r.endDate,
		})
	}
	return edges, nil
}

func validateParentReferences(nodes []nodeCSVRow, rootCode string) error {
	intervals := make(map[string][]nodeCSVRow)
	for _, r := range nodes {
		intervals[r.code] = append(intervals[r.code], r)
	}
	for code := range intervals {
		sort.Slice(intervals[code], func(i, j int) bool {
			return intervals[code][i].effectiveDate.Before(intervals[code][j].effectiveDate)
		})
	}

	parentAt := func(code string, t time.Time) (*string, bool) {
		for _, r := range intervals[code] {
			if !t.Before(r.effectiveDate) && !t.After(r.endDate) {
				if r.parentCode == nil || strings.TrimSpace(*r.parentCode) == "" {
					return nil, true
				}
				p := strings.TrimSpace(*r.parentCode)
				return &p, true
			}
		}
		return nil, false
	}

	for _, r := range nodes {
		if r.code == rootCode {
			continue
		}
		if r.parentCode == nil || strings.TrimSpace(*r.parentCode) == "" {
			return fmt.Errorf("line %d: non-root node must have parent_code", r.line)
		}
		p := strings.TrimSpace(*r.parentCode)

		if _, ok := intervals[p]; !ok {
			return fmt.Errorf("line %d: unknown parent_code: %q", r.line, p)
		}
		_, ok := parentAt(p, r.effectiveDate)
		if !ok {
			return fmt.Errorf("line %d: parent_code %q has no slice covering effective_date %s", r.line, p, r.effectiveDate.Format(time.RFC3339))
		}
	}

	return nil
}

func validateCycles(nodes []nodeCSVRow, edges []edgeRow, strict bool) error {
	intervals := make(map[string][]edgeRow)
	for _, e := range edges {
		intervals[e.childCode] = append(intervals[e.childCode], e)
	}
	for code := range intervals {
		sort.Slice(intervals[code], func(i, j int) bool {
			return intervals[code][i].effectiveDate.Before(intervals[code][j].effectiveDate)
		})
	}

	parentAt := func(code string, t time.Time) (*string, bool) {
		for _, e := range intervals[code] {
			if !t.Before(e.effectiveDate) && !t.After(e.endDate) {
				return e.parentCode, true
			}
		}
		return nil, false
	}

	dates := []time.Time{}
	if strict {
		seen := map[string]struct{}{}
		for _, n := range nodes {
			k := n.effectiveDate.Format(time.RFC3339Nano)
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			dates = append(dates, n.effectiveDate)
		}
		sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
	} else {
		minDate := nodes[0].effectiveDate
		for _, n := range nodes {
			if n.effectiveDate.Before(minDate) {
				minDate = n.effectiveDate
			}
		}
		dates = []time.Time{minDate}
	}

	for _, t := range dates {
		state := map[string]int{} // 0 unvisited, 1 visiting, 2 done
		var visit func(code string) error
		visit = func(code string) error {
			if state[code] == 1 {
				return fmt.Errorf("cycle detected at %s (as-of %s)", code, t.Format(time.RFC3339))
			}
			if state[code] == 2 {
				return nil
			}
			state[code] = 1
			p, ok := parentAt(code, t)
			if ok && p != nil {
				if err := visit(*p); err != nil {
					return err
				}
			}
			state[code] = 2
			return nil
		}
		for code := range intervals {
			if _, ok := parentAt(code, t); !ok {
				continue
			}
			if err := visit(code); err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizePositions(rows []positionCSVRow, nodesByCode map[string]uuid.UUID) ([]positionRow, error) {
	if len(rows) == 0 {
		return nil, nil
	}

	for _, r := range rows {
		switch r.status {
		case "active", "retired", "rescinded":
		default:
			return nil, fmt.Errorf("line %d: invalid status: %q", r.line, r.status)
		}
		if _, ok := nodesByCode[r.orgNodeCode]; !ok {
			return nil, fmt.Errorf("line %d: unknown org_node_code: %q", r.line, r.orgNodeCode)
		}
	}

	byCode := make(map[string][]int)
	for i := range rows {
		byCode[rows[i].code] = append(byCode[rows[i].code], i)
	}
	for code, idxs := range byCode {
		sort.Slice(idxs, func(i, j int) bool {
			return rows[idxs[i]].effectiveDate.Before(rows[idxs[j]].effectiveDate)
		})
		for j := range idxs {
			i := idxs[j]
			if !rows[i].endDateProvided {
				if j+1 < len(idxs) {
					rows[i].endDate = rows[idxs[j+1]].effectiveDate.AddDate(0, 0, -1)
				} else {
					rows[i].endDate = maxEndDate
				}
			}
			if rows[i].effectiveDate.After(rows[i].endDate) {
				return nil, fmt.Errorf("line %d: effective_date must be <= end_date", rows[i].line)
			}
			if j+1 < len(idxs) && !rows[i].endDate.Before(rows[idxs[j+1]].effectiveDate) {
				return nil, fmt.Errorf("line %d: overlapping time windows for position code=%s", rows[i].line, code)
			}
		}
	}

	out := make([]positionRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, positionRow{
			id:            uuid.New(),
			line:          r.line,
			code:          r.code,
			orgNodeCode:   r.orgNodeCode,
			orgNodeID:     nodesByCode[r.orgNodeCode],
			title:         r.title,
			status:        r.status,
			isAutoCreated: r.isAutoCreated,
			effectiveDate: r.effectiveDate,
			endDate:       r.endDate,
		})
	}
	return out, nil
}

func normalizeAssignments(tenantID uuid.UUID, rows []assignmentCSVRow, positions []positionRow) ([]assignmentRow, map[string]uuid.UUID, error) {
	if len(rows) == 0 {
		return nil, map[string]uuid.UUID{}, nil
	}

	positionsByCode := map[string][]positionRow{}
	for _, p := range positions {
		positionsByCode[p.code] = append(positionsByCode[p.code], p)
	}
	for code := range positionsByCode {
		sort.Slice(positionsByCode[code], func(i, j int) bool {
			return positionsByCode[code][i].effectiveDate.Before(positionsByCode[code][j].effectiveDate)
		})
	}

	for _, r := range rows {
		if r.assignmentType != "" && r.assignmentType != "primary" {
			return nil, nil, fmt.Errorf("line %d: assignment_type only supports primary in M1", r.line)
		}
		if _, ok := positionsByCode[r.positionCode]; !ok {
			return nil, nil, fmt.Errorf("line %d: unknown position_code: %q", r.line, r.positionCode)
		}
	}

	byKey := map[string][]int{} // pernr + assignmentType
	for i := range rows {
		key := rows[i].pernr + "|" + rows[i].assignmentType
		byKey[key] = append(byKey[key], i)
	}
	for _, idxs := range byKey {
		sort.Slice(idxs, func(i, j int) bool {
			return rows[idxs[i]].effectiveDate.Before(rows[idxs[j]].effectiveDate)
		})
		for j := range idxs {
			i := idxs[j]
			if !rows[i].endDateProvided {
				if j+1 < len(idxs) {
					rows[i].endDate = rows[idxs[j+1]].effectiveDate.AddDate(0, 0, -1)
				} else {
					rows[i].endDate = maxEndDate
				}
			}
			if rows[i].effectiveDate.After(rows[i].endDate) {
				return nil, nil, fmt.Errorf("line %d: effective_date must be <= end_date", rows[i].line)
			}
			if j+1 < len(idxs) && !rows[i].endDate.Before(rows[idxs[j+1]].effectiveDate) {
				return nil, nil, fmt.Errorf("line %d: overlapping time windows for pernr=%s", rows[i].line, rows[i].pernr)
			}
		}
	}

	resolvePositionID := func(code string, t time.Time) (uuid.UUID, error) {
		slices := positionsByCode[code]
		for _, s := range slices {
			if !t.Before(s.effectiveDate) && !t.After(s.endDate) {
				return s.id, nil
			}
		}
		return uuid.Nil, fmt.Errorf("no position slice for %q covering %s", code, t.Format(time.RFC3339))
	}

	out := make([]assignmentRow, 0, len(rows))
	subjectMappings := map[string]uuid.UUID{}
	for _, r := range rows {
		subjectID := uuid.Nil
		if r.subjectID != nil {
			subjectID = *r.subjectID
		}
		positionID, err := resolvePositionID(r.positionCode, r.effectiveDate)
		if err != nil {
			return nil, nil, fmt.Errorf("line %d: %w", r.line, err)
		}

		subjectMappings[r.pernr] = subjectID
		out = append(out, assignmentRow{
			id:             uuid.New(),
			line:           r.line,
			positionCode:   r.positionCode,
			positionID:     positionID,
			assignmentType: "primary",
			pernr:          r.pernr,
			subjectID:      subjectID,
			effectiveDate:  r.effectiveDate,
			endDate:        r.endDate,
		})
	}

	return out, subjectMappings, nil
}

func dbPrecheckSeed(ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID) error {
	var dummy int
	if err := pool.QueryRow(ctx, `SELECT 1 FROM tenants WHERE id=$1`, tenantID).Scan(&dummy); err != nil {
		if err == pgx.ErrNoRows {
			return withCode(exitValidation, fmt.Errorf("unknown tenant: %s", tenantID))
		}
		return withCode(exitDB, fmt.Errorf("check tenant existence: %w", err))
	}

	var personsOK bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.persons') IS NOT NULL").Scan(&personsOK); err != nil {
		return withCode(exitDB, fmt.Errorf("check persons table: %w", err))
	}
	if !personsOK {
		return withCode(exitValidation, fmt.Errorf("persons table is missing; run `PERSON_MIGRATIONS=1 make db migrate up`"))
	}

	if err := pool.QueryRow(ctx, `SELECT 1 FROM org_nodes WHERE tenant_id=$1 LIMIT 1`, tenantID).Scan(&dummy); err == nil {
		return withCode(exitValidation, fmt.Errorf("seed import requires empty tenant: org_nodes already has rows"))
	} else if err != pgx.ErrNoRows {
		return withCode(exitDB, fmt.Errorf("check org_nodes empty: %w", err))
	}
	return nil
}

func resolveManagers(ctx context.Context, pool *pgxpool.Pool, data *normalizedData) error {
	for i := range data.nodeSlices {
		row := &data.nodeSlices[i]
		if row.managerUserID == nil && (row.managerEmail == nil || strings.TrimSpace(*row.managerEmail) == "") {
			continue
		}

		if row.managerUserID == nil && row.managerEmail != nil {
			var id int64
			err := pool.QueryRow(
				ctx,
				`SELECT id::bigint FROM users WHERE tenant_id=$1 AND lower(email)=lower($2)`,
				data.tenantID,
				*row.managerEmail,
			).Scan(&id)
			if err != nil {
				if err == pgx.ErrNoRows {
					return withCode(exitValidation, fmt.Errorf("line %d: manager_email not found: %s", row.line, *row.managerEmail))
				}
				return withCode(exitDB, fmt.Errorf("line %d: manager_email lookup failed: %w", row.line, err))
			}
			row.managerUserID = &id
		}

		if row.managerUserID != nil {
			var exists int
			err := pool.QueryRow(
				ctx,
				`SELECT 1 FROM users WHERE tenant_id=$1 AND id=$2 LIMIT 1`,
				data.tenantID,
				*row.managerUserID,
			).Scan(&exists)
			if err != nil {
				if err == pgx.ErrNoRows {
					return withCode(exitValidation, fmt.Errorf("line %d: manager_user_id not found: %d", row.line, *row.managerUserID))
				}
				return withCode(exitDB, fmt.Errorf("line %d: manager_user_id lookup failed: %w", row.line, err))
			}
		}
	}
	return nil
}

func resolvePersons(ctx context.Context, pool *pgxpool.Pool, data *normalizedData) error {
	resolved := map[string]uuid.UUID{}
	for pernr, subjectID := range data.subjectMappings {
		pernrTrim := strings.TrimSpace(pernr)
		if pernrTrim == "" {
			continue
		}

		var personUUID uuid.UUID
		err := pool.QueryRow(
			ctx,
			`SELECT person_uuid FROM persons WHERE tenant_id=$1 AND pernr=$2`,
			data.tenantID,
			pernrTrim,
		).Scan(&personUUID)
		if err != nil {
			if err == pgx.ErrNoRows {
				return withCode(exitValidation, fmt.Errorf("pernr not found in persons: %s", pernrTrim))
			}
			return withCode(exitDB, fmt.Errorf("pernr lookup failed: %w", err))
		}

		if subjectID != uuid.Nil && subjectID != personUUID {
			return withCode(exitValidation, fmt.Errorf("subject_id mismatch for pernr=%s", pernrTrim))
		}
		resolved[pernrTrim] = personUUID
	}
	data.subjectMappings = resolved

	for i := range data.assignments {
		pernrTrim := strings.TrimSpace(data.assignments[i].pernr)
		if pernrTrim == "" {
			continue
		}
		if mapped, ok := data.subjectMappings[pernrTrim]; ok && mapped != uuid.Nil {
			data.assignments[i].subjectID = mapped
		}
	}

	return nil
}

type importManifestV1 struct {
	Version    int       `json:"version"`
	RunID      uuid.UUID `json:"run_id"`
	TenantID   uuid.UUID `json:"tenant_id"`
	Mode       string    `json:"mode"`
	Backend    string    `json:"backend"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Input      struct {
		Dir   string            `json:"dir"`
		Files map[string]string `json:"files"`
	} `json:"input"`
	Inserted struct {
		OrgNodes       []uuid.UUID `json:"org_nodes"`
		OrgNodeSlices  []uuid.UUID `json:"org_node_slices"`
		OrgEdges       []uuid.UUID `json:"org_edges"`
		OrgPositions   []uuid.UUID `json:"org_positions"`
		OrgAssignments []uuid.UUID `json:"org_assignments"`
	} `json:"inserted"`
	SubjectMappings []struct {
		Pernr     string    `json:"pernr"`
		SubjectID uuid.UUID `json:"subject_id"`
	} `json:"subject_mappings"`
	Summary map[string]any `json:"summary"`
}

func ensurePositionsImportJobProfileID(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID) (uuid.UUID, error) {
	var jobProfileID uuid.UUID
	if err := tx.QueryRow(ctx, `
	SELECT s.job_profile_id
	FROM org_job_profile_slices s
	JOIN org_job_profile_slice_job_families sf
		ON sf.tenant_id=s.tenant_id
		AND sf.job_profile_slice_id=s.id
		AND sf.is_primary=TRUE
	WHERE s.tenant_id=$1
	ORDER BY s.is_active DESC, s.effective_date ASC, s.job_profile_id ASC
	LIMIT 1
	`, tenantID).Scan(&jobProfileID); err == nil {
		return jobProfileID, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, err
	}

	baselineDate := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

	groupCode := "IMPORT"
	groupName := "导入默认职类"
	if _, err := tx.Exec(ctx, `
	INSERT INTO org_job_family_groups (tenant_id, code, name, is_active)
	VALUES ($1,$2,$3,TRUE)
	ON CONFLICT (tenant_id, code) DO NOTHING
	`, tenantID, groupCode, groupName); err != nil {
		return uuid.Nil, err
	}

	var groupID uuid.UUID
	if err := tx.QueryRow(ctx, `
	SELECT id
	FROM org_job_family_groups
	WHERE tenant_id=$1 AND code=$2
	LIMIT 1
	`, tenantID, groupCode).Scan(&groupID); err != nil {
		return uuid.Nil, err
	}

	var hasGroupSlice bool
	if err := tx.QueryRow(ctx, `
	SELECT EXISTS (
		SELECT 1
		FROM org_job_family_group_slices
		WHERE tenant_id=$1 AND job_family_group_id=$2
	)
	`, tenantID, groupID).Scan(&hasGroupSlice); err != nil {
		return uuid.Nil, err
	}
	if !hasGroupSlice {
		if _, err := tx.Exec(ctx, `
		INSERT INTO org_job_family_group_slices
			(tenant_id, job_family_group_id, name, is_active, effective_date, end_date)
		VALUES (
			$1,$2,$3,TRUE,
			($4 AT TIME ZONE 'UTC')::date,
			($5 AT TIME ZONE 'UTC')::date
		)
		`, tenantID, groupID, groupName, baselineDate, endDate); err != nil {
			return uuid.Nil, err
		}
	}

	familyCode := "IMPORT"
	familyName := "导入默认职种"
	if _, err := tx.Exec(ctx, `
	INSERT INTO org_job_families (tenant_id, job_family_group_id, code, name, is_active)
	VALUES ($1,$2,$3,$4,TRUE)
	ON CONFLICT (tenant_id, job_family_group_id, code) DO NOTHING
	`, tenantID, groupID, familyCode, familyName); err != nil {
		return uuid.Nil, err
	}

	var familyID uuid.UUID
	if err := tx.QueryRow(ctx, `
	SELECT id
	FROM org_job_families
	WHERE tenant_id=$1 AND job_family_group_id=$2 AND code=$3
	LIMIT 1
	`, tenantID, groupID, familyCode).Scan(&familyID); err != nil {
		return uuid.Nil, err
	}

	var hasFamilySlice bool
	if err := tx.QueryRow(ctx, `
	SELECT EXISTS (
		SELECT 1
		FROM org_job_family_slices
		WHERE tenant_id=$1 AND job_family_id=$2
	)
	`, tenantID, familyID).Scan(&hasFamilySlice); err != nil {
		return uuid.Nil, err
	}
	if !hasFamilySlice {
		if _, err := tx.Exec(ctx, `
		INSERT INTO org_job_family_slices
			(tenant_id, job_family_id, name, is_active, effective_date, end_date)
		VALUES (
			$1,$2,$3,TRUE,
			($4 AT TIME ZONE 'UTC')::date,
			($5 AT TIME ZONE 'UTC')::date
		)
		`, tenantID, familyID, familyName, baselineDate, endDate); err != nil {
			return uuid.Nil, err
		}
	}

	profileCode := "IMPORT-DEFAULT"
	profileName := "导入默认职位模板"
	if _, err := tx.Exec(ctx, `
	INSERT INTO org_job_profiles (tenant_id, code, name, is_active)
	VALUES ($1,$2,$3,TRUE)
	ON CONFLICT (tenant_id, code) DO NOTHING
	`, tenantID, profileCode, profileName); err != nil {
		return uuid.Nil, err
	}

	if err := tx.QueryRow(ctx, `
	SELECT id
	FROM org_job_profiles
	WHERE tenant_id=$1 AND code=$2
	LIMIT 1
	`, tenantID, profileCode).Scan(&jobProfileID); err != nil {
		return uuid.Nil, err
	}

	var profileSliceID uuid.UUID
	if err := tx.QueryRow(ctx, `
	SELECT id
	FROM org_job_profile_slices
	WHERE tenant_id=$1
		AND job_profile_id=$2
		AND effective_date <= ($3 AT TIME ZONE 'UTC')::date
		AND end_date >= ($3 AT TIME ZONE 'UTC')::date
	ORDER BY effective_date DESC
	LIMIT 1
	`, tenantID, jobProfileID, baselineDate).Scan(&profileSliceID); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, err
		}
		if err := tx.QueryRow(ctx, `
		SELECT id
		FROM org_job_profile_slices
		WHERE tenant_id=$1 AND job_profile_id=$2
		ORDER BY effective_date ASC
		LIMIT 1
		`, tenantID, jobProfileID).Scan(&profileSliceID); err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return uuid.Nil, err
			}
			if err := tx.QueryRow(ctx, `
			INSERT INTO org_job_profile_slices
				(tenant_id, job_profile_id, name, description, is_active, external_refs, effective_date, end_date)
			VALUES (
				$1,$2,$3,NULL,TRUE,'{}'::jsonb,
				($4 AT TIME ZONE 'UTC')::date,
				($5 AT TIME ZONE 'UTC')::date
			)
			RETURNING id
			`, tenantID, jobProfileID, profileName, baselineDate, endDate).Scan(&profileSliceID); err != nil {
				return uuid.Nil, err
			}
		}
	}

	var primaryCount int
	if err := tx.QueryRow(ctx, `
	SELECT COALESCE(SUM(CASE WHEN is_primary THEN 1 ELSE 0 END), 0)::int
	FROM org_job_profile_slice_job_families
	WHERE tenant_id=$1 AND job_profile_slice_id=$2
	`, tenantID, profileSliceID).Scan(&primaryCount); err != nil {
		return uuid.Nil, err
	}
	if primaryCount != 1 {
		if _, err := tx.Exec(ctx, `
		DELETE FROM org_job_profile_slice_job_families
		WHERE tenant_id=$1 AND job_profile_slice_id=$2
		`, tenantID, profileSliceID); err != nil {
			return uuid.Nil, err
		}
		if _, err := tx.Exec(ctx, `
		INSERT INTO org_job_profile_slice_job_families (tenant_id, job_profile_slice_id, job_family_id, is_primary)
		VALUES ($1,$2,$3,TRUE)
		`, tenantID, profileSliceID, familyID); err != nil {
			return uuid.Nil, err
		}
	}

	return jobProfileID, nil
}

func applySeedImport(ctx context.Context, pool *pgxpool.Pool, data normalizedData, opts importOptions) (*importManifestV1, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return nil, withCode(exitDB, fmt.Errorf("begin tx: %w", err))
	}
	defer func() { _ = tx.Rollback(ctx) }()

	txCtx := composables.WithTenantID(ctx, data.tenantID)
	if err := composables.ApplyTenantRLS(txCtx, tx); err != nil {
		return nil, withCode(exitDB, err)
	}

	manifest := &importManifestV1{
		Version:    1,
		RunID:      data.runID,
		TenantID:   data.tenantID,
		Mode:       opts.mode,
		Backend:    opts.backend,
		StartedAt:  data.startedAt,
		FinishedAt: time.Time{},
		Summary:    map[string]any{},
	}
	manifest.Input.Dir = opts.inputDir
	manifest.Input.Files = map[string]string{
		"nodes": "nodes.csv",
	}
	if _, err := os.Stat(filepath.Join(opts.inputDir, "positions.csv")); err == nil {
		manifest.Input.Files["positions"] = "positions.csv"
	}
	if !opts.skipAssignments {
		if _, err := os.Stat(filepath.Join(opts.inputDir, "assignments.csv")); err == nil {
			manifest.Input.Files["assignments"] = "assignments.csv"
		}
	}

	rootCode := findRootCode(data.nodeSlices)
	for code, nodeID := range data.nodesByCode {
		isRoot := code == rootCode
		if _, err := tx.Exec(
			txCtx,
			`INSERT INTO org_nodes (tenant_id, id, type, code, is_root) VALUES ($1,$2,$3,$4,$5)`,
			data.tenantID, nodeID, "OrgUnit", code, isRoot,
		); err != nil {
			return nil, withCode(exitDBWrite, fmt.Errorf("insert org_nodes(%s): %w", code, err))
		}
		manifest.Inserted.OrgNodes = append(manifest.Inserted.OrgNodes, nodeID)
	}
	sort.Slice(manifest.Inserted.OrgNodes, func(i, j int) bool {
		return manifest.Inserted.OrgNodes[i].String() < manifest.Inserted.OrgNodes[j].String()
	})

	type nodeSliceInsert struct {
		id  uuid.UUID
		row nodeCSVRow
	}
	nodeSliceIDs := make([]uuid.UUID, 0, len(data.nodeSlices))
	nodeSliceInserts := make([]nodeSliceInsert, 0, len(data.nodeSlices))
	for _, r := range data.nodeSlices {
		nodeSliceInserts = append(nodeSliceInserts, nodeSliceInsert{
			id:  uuid.New(),
			row: r,
		})
	}
	for _, ins := range nodeSliceInserts {
		r := ins.row
		nodeID := data.nodesByCode[r.code]

		var parentHint any = nil
		if r.parentCode != nil && strings.TrimSpace(*r.parentCode) != "" {
			parentHint = data.nodesByCode[strings.TrimSpace(*r.parentCode)]
		}

		if _, err := tx.Exec(
			txCtx,
			`INSERT INTO org_node_slices (
						tenant_id, id, org_node_id, name, i18n_names, status, legal_entity_id, company_code, location_id,
						display_order, parent_hint, manager_user_id, effective_date, end_date
					) VALUES (
						$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14
					)`,
			data.tenantID,
			ins.id,
			nodeID,
			r.name,
			[]byte(r.i18nNames),
			r.status,
			r.legalEntityID,
			r.companyCode,
			r.locationID,
			r.displayOrder,
			parentHint,
			r.managerUserID,
			pgDateOnlyUTC(r.effectiveDate),
			pgDateOnlyUTC(r.endDate),
		); err != nil {
			return nil, withCode(exitDBWrite, fmt.Errorf("line %d: insert org_node_slices(%s): %w", r.line, r.code, err))
		}
		nodeSliceIDs = append(nodeSliceIDs, ins.id)
	}
	manifest.Inserted.OrgNodeSlices = nodeSliceIDs

	edgeIDs := make([]uuid.UUID, 0, len(data.edges))
	if err := insertEdges(txCtx, tx, data); err != nil {
		return nil, err
	}
	for _, e := range data.edges {
		edgeIDs = append(edgeIDs, e.id)
	}
	manifest.Inserted.OrgEdges = edgeIDs

	jobProfileID, err := ensurePositionsImportJobProfileID(txCtx, tx, data.tenantID)
	if err != nil {
		return nil, withCode(exitDBWrite, fmt.Errorf("ensure import job profile: %w", err))
	}

	for _, p := range data.positions {
		sliceID := uuid.New()
		if _, err := tx.Exec(
			txCtx,
			`INSERT INTO org_positions (
							tenant_id, id, org_node_id, code, title, status, is_auto_created, effective_date, end_date
						) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			data.tenantID, p.id, p.orgNodeID, p.code, p.title, p.status, p.isAutoCreated, pgDateOnlyUTC(p.effectiveDate), pgDateOnlyUTC(p.endDate),
		); err != nil {
			return nil, withCode(exitDBWrite, fmt.Errorf("line %d: insert org_positions(%s): %w", p.line, p.code, err))
		}
		if _, err := tx.Exec(
			txCtx,
			`INSERT INTO org_position_slices (
								tenant_id, id, position_id, org_node_id, title, lifecycle_status, capacity_fte, job_profile_id, effective_date, end_date
							) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			data.tenantID, sliceID, p.id, p.orgNodeID, p.title, p.status, 1.0, jobProfileID, pgDateOnlyUTC(p.effectiveDate), pgDateOnlyUTC(p.endDate),
		); err != nil {
			return nil, withCode(exitDBWrite, fmt.Errorf("line %d: insert org_position_slices(%s): %w", p.line, p.code, err))
		}
		if _, err := tx.Exec(txCtx, `
					INSERT INTO org_position_slice_job_families (tenant_id, position_slice_id, job_family_id, is_primary)
					SELECT $1, $3, sf.job_family_id, sf.is_primary
					FROM (
						SELECT id
						FROM org_job_profile_slices
						WHERE tenant_id=$1
							AND job_profile_id=$2
							AND effective_date <= $4::date
							AND end_date >= $4::date
						ORDER BY effective_date DESC
						LIMIT 1
					) s
					JOIN org_job_profile_slice_job_families sf
						ON sf.tenant_id=$1
						AND sf.job_profile_slice_id=s.id
					ON CONFLICT (tenant_id, position_slice_id, job_family_id) DO NOTHING
					`, data.tenantID, jobProfileID, sliceID, pgDateOnlyUTC(p.effectiveDate)); err != nil {
			return nil, withCode(exitDBWrite, fmt.Errorf("line %d: insert org_position_slice_job_families(%s): %w", p.line, p.code, err))
		}
		manifest.Inserted.OrgPositions = append(manifest.Inserted.OrgPositions, p.id)
	}

	for _, a := range data.assignments {
		if _, err := tx.Exec(
			txCtx,
			`INSERT INTO org_assignments (
						tenant_id, id, position_id, subject_id, pernr, is_primary, effective_date, end_date
					) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			data.tenantID, a.id, a.positionID, a.subjectID, a.pernr, true, pgDateOnlyUTC(a.effectiveDate), pgDateOnlyUTC(a.endDate),
		); err != nil {
			return nil, withCode(exitDBWrite, fmt.Errorf("line %d: insert org_assignments(pernr=%s): %w", a.line, a.pernr, err))
		}
		manifest.Inserted.OrgAssignments = append(manifest.Inserted.OrgAssignments, a.id)
	}

	for pernr, sid := range data.subjectMappings {
		manifest.SubjectMappings = append(manifest.SubjectMappings, struct {
			Pernr     string    `json:"pernr"`
			SubjectID uuid.UUID `json:"subject_id"`
		}{Pernr: pernr, SubjectID: sid})
	}
	sort.Slice(manifest.SubjectMappings, func(i, j int) bool { return manifest.SubjectMappings[i].Pernr < manifest.SubjectMappings[j].Pernr })

	manifest.Summary["nodes_rows"] = len(data.nodeSlices)
	manifest.Summary["positions_rows"] = len(data.positions)
	manifest.Summary["assignments_rows"] = len(data.assignments)

	if err := tx.Commit(ctx); err != nil {
		return nil, withCode(exitDB, fmt.Errorf("commit tx: %w", err))
	}
	manifest.FinishedAt = time.Now().UTC()
	return manifest, nil
}

func insertEdges(ctx context.Context, tx pgx.Tx, data normalizedData) error {
	byDate := map[string][]edgeRow{}
	dates := []time.Time{}
	seen := map[string]struct{}{}
	for _, e := range data.edges {
		k := e.effectiveDate.Format(time.RFC3339Nano)
		byDate[k] = append(byDate[k], e)
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			dates = append(dates, e.effectiveDate)
		}
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })

	parentAt := func(code string, t time.Time) (*string, bool) {
		for _, e := range data.edges {
			if e.childCode != code {
				continue
			}
			if !t.Before(e.effectiveDate) && !t.After(e.endDate) {
				return e.parentCode, true
			}
		}
		return nil, false
	}

	for _, t := range dates {
		k := t.Format(time.RFC3339Nano)
		es := byDate[k]
		depth := map[string]int{}
		var calcDepth func(code string) int
		calcDepth = func(code string) int {
			if d, ok := depth[code]; ok {
				return d
			}
			p, ok := parentAt(code, t)
			if !ok || p == nil {
				depth[code] = 0
				return 0
			}
			d := calcDepth(*p) + 1
			depth[code] = d
			return d
		}

		sort.SliceStable(es, func(i, j int) bool {
			di := calcDepth(es[i].childCode)
			dj := calcDepth(es[j].childCode)
			if di != dj {
				return di < dj
			}
			return es[i].childCode < es[j].childCode
		})

		for _, e := range es {
			childID := data.nodesByCode[e.childCode]
			var parentID any = nil
			if e.parentCode != nil {
				parentID = data.nodesByCode[*e.parentCode]
			}
			if _, err := tx.Exec(
				ctx,
				`INSERT INTO org_edges (tenant_id, id, hierarchy_type, parent_node_id, child_node_id, effective_date, end_date)
						 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
				data.tenantID, e.id, "OrgUnit", parentID, childID, pgDateOnlyUTC(e.effectiveDate), pgDateOnlyUTC(e.endDate),
			); err != nil {
				return withCode(exitDBWrite, fmt.Errorf("insert org_edges(child=%s): %w", e.childCode, err))
			}
		}
	}

	return nil
}

func findRootCode(nodes []nodeCSVRow) string {
	for _, r := range nodes {
		if r.parentCode == nil || strings.TrimSpace(*r.parentCode) == "" {
			return r.code
		}
	}
	return ""
}

func writeManifest(outputDir string, manifest *importManifestV1) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	ts := time.Now().UTC().Format("20060102T150405Z")
	name := fmt.Sprintf("import_manifest_%s_%s.json", ts, manifest.RunID.String())
	path := filepath.Join(outputDir, name)

	b, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

type importSummary struct {
	Status          string `json:"status"`
	RunID           string `json:"run_id"`
	TenantID        string `json:"tenant_id"`
	Backend         string `json:"backend"`
	Mode            string `json:"mode"`
	Apply           bool   `json:"apply"`
	InputDir        string `json:"input_dir"`
	OutputDir       string `json:"output_dir"`
	ManifestVersion *int   `json:"manifest_version,omitempty"`
	Counts          struct {
		NodesRows       int `json:"nodes_rows"`
		PositionsRows   int `json:"positions_rows"`
		AssignmentsRows int `json:"assignments_rows"`
	} `json:"counts"`
}

func printImportSummary(data normalizedData, opts importOptions, status string, manifest *importManifestV1) error {
	var s importSummary
	s.Status = status
	s.RunID = data.runID.String()
	s.TenantID = data.tenantID.String()
	s.Backend = opts.backend
	s.Mode = opts.mode
	s.Apply = opts.apply
	s.InputDir = opts.inputDir
	s.OutputDir = opts.outputDir
	s.Counts.NodesRows = len(data.nodeSlices)
	s.Counts.PositionsRows = len(data.positions)
	s.Counts.AssignmentsRows = len(data.assignments)
	if manifest != nil {
		s.ManifestVersion = &manifest.Version
	}
	return writeJSONLine(s)
}
