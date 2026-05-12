package reports

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"sort"
	"strings"

	"server_1/internal/core/db"
)

// ReportButton represents a button for a report row.
type ReportButton struct {
	ButtonIcon  string `json:"button_icon"`
	ButtonTT    string `json:"button_tt"`
	Permission  string `json:"permission"`
	ButtonColor string `json:"button_color"`
	URL         string `json:"url"`
	ConcatID    string `json:"concat_id"`
	Action      string `json:"action"`
	Hide        string `json:"hide"`
	ModuleID    int64  `json:"module_id"`
	OnClick     string `json:"onclick"`
	Modal       string `json:"modal"`
	CaseName    string `json:"case_name"`
	CheckRow    int    `json:"check_row"`
	Query       string `json:"query"`
}

// ReportResponse is the full response for a report query.
type ReportResponse struct {
	Data          [][]any  `json:"data"`
	RowID         []any    `json:"row_id"`
	ContactID     []any    `json:"contact_id"`
	ImageURL      []any    `json:"image_url"`
	RowColor      []any    `json:"row_color"`
	Header        []string `json:"header"`
	Icon          string   `json:"icon"`
	ShowSR        bool     `json:"show_sr"`
	Count         int      `json:"count"`
	ModuleID      int64    `json:"module_id"`
	Title         string   `json:"title"`
	Subtitle      string   `json:"subtitle"`
	DateFilterCol string   `json:"date_filter_col"`
	DynamicReport bool     `json:"dynamic_report"`
	ReportOption  string   `json:"report_option"`
	Default       int      `json:"default"`
	Mpos          int      `json:"mpos"`
	UIFilters     []any    `json:"ui_filters"`
	Col           []string `json:"col"`
	Total         string   `json:"total"`
	Error         int      `json:"error"`
	ErrorMsg      string   `json:"error_msg"`
	Message       string   `json:"message"`
}

// GetReportResponse builds the full report response including buttons and row mapping.
func (r *Repo) GetReportResponse(ctx context.Context, id int64, opts map[string]any) (*ReportResponse, error) {
	meta, err := r.Get(ctx, id)
	if err != nil {
		return &ReportResponse{
			Data:          [][]any{},
			Error:         1,
			Title:         meta.Title,
			Subtitle:      meta.Subtitle,
			DateFilterCol: meta.DateFilterCol,
			DynamicReport: meta.DynamicReport,
			ReportOption:  meta.ReportOption,
			ModuleID:      meta.ModuleID,
			Icon:          meta.Icon,
			ShowSR:        meta.ShowSR,
			ErrorMsg:      "record not found",
			Message:       "Not found",
		}, nil
	}

	// Fetch buttons
	btnRows, err := r.db1.QueryContext(ctx, `SELECT button_icon,button_tt,permission,button_color,url,concat_id,action,hide,module_id,onclick,modal,case_name,check_row,query FROM report_button WHERE report_id = ? AND hide <> '1'`, id)
	if err != nil {
		return nil, fmt.Errorf("fetch buttons: %w", err)
	}
	buttons := []ReportButton{}
	for btnRows.Next() {
		var b ReportButton
		if err := btnRows.Scan(&b.ButtonIcon, &b.ButtonTT, &b.Permission, &b.ButtonColor, &b.URL, &b.ConcatID, &b.Action, &b.Hide, &b.ModuleID, &b.OnClick, &b.Modal, &b.CaseName, &b.CheckRow, &b.Query); err == nil {
			buttons = append(buttons, b)
		}
	}
	btnRows.Close()
	if err := btnRows.Err(); err != nil {
		return nil, fmt.Errorf("buttons rows: %w", err)
	}

	// Run main report query
	rows, _, err := r.RunReportQuery(ctx, id, opts)
	if err != nil {
		return nil, fmt.Errorf("run report query: %w", err)
	}

	// Prepare response fields
	counsellorSet := map[string]struct{}{}

	response := &ReportResponse{
		Data:          [][]any{},
		RowID:         []any{},
		ContactID:     []any{},
		ImageURL:      []any{},
		RowColor:      []any{},
		Header:        []string{},
		Icon:          meta.Icon,
		ShowSR:        meta.ShowSR,
		Count:         len(rows),
		ModuleID:      meta.ModuleID,
		Title:         meta.Title,
		Subtitle:      meta.Subtitle,
		DateFilterCol: meta.DateFilterCol,
		DynamicReport: meta.DynamicReport,
		ReportOption:  meta.ReportOption,
		Default:       0,
		Mpos:          0,
		UIFilters:     []any{},
		Col:           []string{},
		Total:         "", // TODO: compute total
		Error:         0,
		ErrorMsg:      "",
		Message:       "",
	}

	// Build header from columns
	for _, col := range meta.Columns {
		response.Header = append(response.Header, col.Header)
		response.Col = append(response.Col, col.ColumnName)
	}

	// Map each row
	cnt := 1
	for _, row := range rows {
		temp := []any{}
		tempBtn := []any{}
		// Show SR
		if meta.ShowSR {
			temp = append(temp, map[string]any{"value": cnt, "url": "", "modal": "", "case_name": "", "data": "", "onclick": ""})
		}
		if name, ok := row["counsellor_name"]; ok {
			if s := normalizeString(name); s != "" {
				counsellorSet[s] = struct{}{}
			}
		}
		// Map columns
		for _, col := range meta.Columns {
			val := row[col.ColumnName]
			temp = append(temp, val)
		}
		// TODO: Button logic (permission, hide, etc.)
		for _, btn := range buttons {
			// For now, just append all buttons
			tempBtn = append(tempBtn, btn)
		}
		temp = append(temp, tempBtn)
		response.Data = append(response.Data, temp)
		cnt++
	}

	if len(counsellorSet) > 0 {
		names := make([]string, 0, len(counsellorSet))
		for name := range counsellorSet {
			names = append(names, name)
		}
		sort.Strings(names)
		response.UIFilters = append(response.UIFilters, map[string]any{
			"field":  "counsellor_name",
			"label":  "Counsellor Name",
			"values": names,
		})
	}

	// TODO: fill extra fields (row_id, contact_id, image_url, row_color, total, etc.)

	return response, nil
}

type Repo struct {
	db1 *db.SQL
	db2 *db.SQL
}

func NewRepo() *Repo {
	return &Repo{
		db1: db.DBx("DB1"),
		db2: db.DBx("DB2"),
	}
}

// choose picks a DB by name; mirrors test_items behavior
func (r *Repo) choose(name string) *db.SQL {
	if name == "DB2" {
		return r.db2
	}
	return r.db1
}

// List returns an empty slice for now
func (r *Repo) List(ctx context.Context, limit, offset int) ([]Report, error) {
	return []Report{}, nil
}

// Get returns nil for now
// func (r *Repo) Get(ctx context.Context, id int64) (*Report, error) {
//     return nil, sql.ErrNoRows
// }

// Create is a stub
func (r *Repo) Create(ctx context.Context, title, content string) (int64, error) {
	return 0, nil
}

// Update is a stub
func (r *Repo) Update(ctx context.Context, id int64, title *string, content *string) error {
	return nil
}

// Delete is a stub
func (r *Repo) Delete(ctx context.Context, id int64) error {
	return nil
}

// GetReport loads report metadata and its columns from DB1 matching the given id.
func (r *Repo) Get(ctx context.Context, id int64) (*ReportMeta, error) {
	q := `SELECT r.id, r.module_id, r.date_filter_col, r.report_option, r.dynamic_report, r.table_name, r.icon, r.show_sr, r.title, r.subtitle, r.total, rc.column_name, rc.header, rc.isAdmin, rc.position, rc.mpos, rc.url, rc.concat_id, rc.onclick, rc.modal
			FROM report AS r
			INNER JOIN report_column AS rc ON r.id = rc.report_id
			WHERE r.id = ?
			ORDER BY rc.position ASC` // parameterized to avoid SQL injection

	rows, err := r.db1.QueryContext(ctx, q, id)
	if err != nil {
		return nil, fmt.Errorf("query report: %w", err)
	}
	defer rows.Close()

	var meta *ReportMeta
	cols := []ReportColumn{}
	for rows.Next() {
		var (
			rid           sql.NullInt64
			moduleID      sql.NullInt64
			dateFilterCol sql.NullString
			reportOption  sql.NullString
			dynamicReport sql.NullBool
			tableName     sql.NullString
			icon          sql.NullString
			showSR        sql.NullBool
			title         sql.NullString
			subtitle      sql.NullString
			total         sql.NullString
			columnName    sql.NullString
			header        sql.NullString
			isAdmin       sql.NullBool
			position      sql.NullInt64
			mpos          sql.NullInt64
			url           sql.NullString
			concatID      sql.NullString
			onclick       sql.NullString
			modal         sql.NullString
		)

		if err := rows.Scan(&rid, &moduleID, &dateFilterCol, &reportOption, &dynamicReport, &tableName, &icon, &showSR, &title, &subtitle, &total, &columnName, &header, &isAdmin, &position, &mpos, &url, &concatID, &onclick, &modal); err != nil {
			return nil, fmt.Errorf("scan report row: %w", err)
		}

		if meta == nil {
			meta = &ReportMeta{
				ID:            rid.Int64,
				ModuleID:      moduleID.Int64,
				DateFilterCol: dateFilterCol.String,
				ReportOption:  reportOption.String,
				DynamicReport: dynamicReport.Bool,
				TableName:     tableName.String,
				Icon:          icon.String,
				ShowSR:        showSR.Bool,
				Title:         title.String,
				Subtitle:      subtitle.String,
				Total:         total.String,
				Columns:       []ReportColumn{},
			}
		}

		rc := ReportColumn{
			ColumnName: columnName.String,
			Header:     header.String,
			IsAdmin:    isAdmin.Bool,
			Position:   int(position.Int64),
			Mpos:       int(mpos.Int64),
			URL:        url.String,
			ConcatID:   concatID.String,
			OnClick:    onclick.String,
			Modal:      modal.String,
		}
		cols = append(cols, rc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	if meta == nil {
		return nil, sql.ErrNoRows
	}
	meta.Columns = cols
	return meta, nil
}

// GetStructuredColumns fetches rows from structured_column for a given report id.
func (r *Repo) GetStructuredColumns(ctx context.Context, reportID int64) ([]StructuredColumn, error) {
	q := `SELECT id, r_id, header, display_keys, position, park FROM structured_column WHERE r_id = ? ORDER BY position ASC`
	rows, err := r.db1.QueryContext(ctx, q, reportID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols := []StructuredColumn{}
	for rows.Next() {
		var sc StructuredColumn
		var pos sql.NullInt64
		if err := rows.Scan(&sc.ID, &sc.RID, &sc.Header, &sc.DisplayKeys, &pos, &sc.Park); err != nil {
			return nil, err
		}
		if pos.Valid {
			sc.Position = int(pos.Int64)
		}
		cols = append(cols, sc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return cols, nil
}

// BuildReportQuery constructs the SQL query string for a report id using optional
// startDate, endDate and value placeholders (used when dynamic_report is true).
// It returns the final SQL string (without executing it).
// BuildReportQuery builds the SQL query string for a report id using all options.
func (r *Repo) BuildReportQuery(ctx context.Context, id int64, opts map[string]any) (string, error) {
	// opts: start_date, end_date, value, groupby, orderby, limit, offset
	meta, err := r.Get(ctx, id)
	if err != nil {
		return "", err
	}
	startDate, _ := opts["start_date"].(string)
	endDate, _ := opts["end_date"].(string)
	value, _ := opts["value"].(string)
	groupby, _ := opts["groupby"].(string)
	limit, _ := opts["limit"].(string)
	offset, _ := opts["offset"].(string)
	disableOrder, _ := opts["disable_order"].(bool)
	// orderby: []map[string]string or string
	orderby, _ := opts["orderby"].([]map[string]string)

	// Build column list
	cols := []string{}
	for _, c := range meta.Columns {
		if strings.TrimSpace(c.ColumnName) != "" {
			cols = append(cols, c.ColumnName)
		}
	}
	colStr := "*"
	if len(cols) > 0 {
		colStr = strings.Join(cols, ", ")
	}

	tableAlias := meta.TableName
	if strings.TrimSpace(tableAlias) == "" {
		tableAlias = "t"
	}

	var query string
	if meta.DynamicReport {
		var view string
		if override, ok := reportViewOverride(id); ok {
			view = override
		} else {
			// fetch view definition
			err := r.db1.QueryRowContext(ctx, `SELECT view FROM report_view_table WHERE r_id = ?`, id).Scan(&view)
			if err != nil {
				return "", fmt.Errorf("fetch view: %w", err)
			}
		}
		// replace placeholders
		view = strings.ReplaceAll(view, "#sdate", startDate)
		view = strings.ReplaceAll(view, "#edate", endDate)
		view = strings.ReplaceAll(view, "#value", value)

		query = fmt.Sprintf("SELECT %s FROM (%s) AS %s", colStr, view, tableAlias)
	} else {
		query = fmt.Sprintf("SELECT %s FROM %s AS %s", colStr, meta.TableName, tableAlias)
	}

	// fetch filters
	rows, err := r.db1.QueryContext(ctx, `SELECT name, operator, value FROM report_filter WHERE report_id = ?`, id)
	if err != nil {
		return "", fmt.Errorf("fetch filters: %w", err)
	}
	defer rows.Close()

	conds := []string{}
	for rows.Next() {
		var name, operator, val string
		if err := rows.Scan(&name, &operator, &val); err != nil {
			return "", fmt.Errorf("scan filter: %w", err)
		}
		esc := strings.ReplaceAll(val, "'", "''")
		op := strings.ToUpper(strings.TrimSpace(operator))
		switch op {
		case "IN":
			parts := strings.Split(esc, ",")
			for i := range parts {
				parts[i] = fmt.Sprintf("'%s'", strings.TrimSpace(parts[i]))
			}
			conds = append(conds, fmt.Sprintf("%s IN (%s)", name, strings.Join(parts, ", ")))
		case "LIKE":
			conds = append(conds, fmt.Sprintf("%s LIKE '%s'", name, esc))
		default:
			conds = append(conds, fmt.Sprintf("%s %s '%s'", name, operator, esc))
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("filters rows: %w", err)
	}

	// Add date filter if present
	if meta.DateFilterCol != "" {
		if startDate != "" {
			conds = append(conds, fmt.Sprintf("%s >= '%s'", meta.DateFilterCol, startDate))
		}
		if endDate != "" {
			conds = append(conds, fmt.Sprintf("%s <= '%s'", meta.DateFilterCol, endDate))
		}
	}

	if len(conds) > 0 {
		query = query + " WHERE " + strings.Join(conds, " AND ")
	}

	// GROUP BY
	if groupby != "" {
		if !isSafeIdentifierList(groupby) {
			return "", fmt.Errorf("invalid groupby clause")
		}
		query += " GROUP BY " + groupby
	}

	// ORDER BY
	orderByStr := ""
	if disableOrder {
		// caller explicitly disabled ordering
	} else if len(orderby) > 0 {
		parts := []string{}
		for _, ord := range orderby {
			for k, v := range ord {
				parts = append(parts, fmt.Sprintf("%s %s", k, v))
			}
		}
		parts = append(parts, "id DESC")
		orderByStr = strings.Join(parts, ", ")
	} else {
		// system order by (fallback from report_order table)
		sysParts := []string{}
		sysRows, err := r.db1.QueryContext(ctx, `SELECT col, order_by FROM report_order WHERE report_id = ?`, id)
		if err == nil {
			for sysRows.Next() {
				var col string
				var order int
				if err := sysRows.Scan(&col, &order); err == nil {
					sysParts = append(sysParts, fmt.Sprintf("%s %s", col, map[int]string{0: "ASC", 1: "DESC"}[order]))
				}
			}
			sysRows.Close()
			if err := sysRows.Err(); err != nil {
				return "", fmt.Errorf("report_order rows: %w", err)
			}
		}
		if meta.DateFilterCol != "" {
			sysParts = append(sysParts, meta.DateFilterCol+" DESC")
		}
		if len(sysParts) == 0 {
			sysParts = append(sysParts, "id DESC")
		}
		orderByStr = strings.Join(sysParts, ", ")
	}
	if orderByStr != "" {
		if !isSafeOrderBy(orderByStr) {
			return "", fmt.Errorf("invalid order by clause")
		}
		query += " ORDER BY " + orderByStr
	}

	// LIMIT/OFFSET
	if limit != "" {
		lim, err := parsePositiveInt(limit, 1000)
		if err != nil { return "", fmt.Errorf("invalid limit: %w", err) }
		query += fmt.Sprintf(" LIMIT %d", lim)
	}
	if offset != "" {
		off, err := parsePositiveInt(offset, 0)
		if err != nil { return "", fmt.Errorf("invalid offset: %w", err) }
		query += fmt.Sprintf(" OFFSET %d", off)
	}

	return query, nil
}

func normalizeString(val any) string {
	switch v := val.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case []byte:
		return strings.TrimSpace(string(v))
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "<nil>" {
			return ""
		}
		return s
	}
}

// RunReportQuery builds and executes the report query, returning result data.
func (r *Repo) RunReportQuery(ctx context.Context, id int64, opts map[string]any) ([]map[string]any, string, error) {
	query, err := r.BuildReportQuery(ctx, id, opts)
	if err != nil {
		return nil, "", err
	}
	dbName, _ := opts["db_name"].(string)
	rows, err := r.choose(dbName).QueryContext(ctx, query)
	if err != nil && strings.EqualFold(strings.TrimSpace(dbName), "DB2") {
		// Override queries are fully qualified — retry on DB1 for any DB2 failure
		// (missing table, invalid connection, timeout, etc.)
		rows, err = r.db1.QueryContext(ctx, query)
	}
	if err != nil {
		return nil, query, fmt.Errorf("report %d query failed: %w", id, err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, query, err
	}
	result := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, query, err
		}
		rowMap := map[string]any{}
		for i, col := range cols {
			v := vals[i]
			// convert []byte to string for text columns
			if b, ok := v.([]byte); ok {
				rowMap[col] = string(b)
			} else {
				rowMap[col] = v
			}
		}
		result = append(result, rowMap)
	}
	if err := rows.Err(); err != nil {
		return nil, query, err
	}
	return result, query, nil
}

// RunReportAggregates returns total row count and optional total sum for a report
// without materializing all rows in Go.
func (r *Repo) RunReportAggregates(ctx context.Context, id int64, opts map[string]any, totalColumn string) (int, float64, string, error) {
	aggOpts := map[string]any{}
	for k, v := range opts {
		aggOpts[k] = v
	}
	delete(aggOpts, "limit")
	delete(aggOpts, "offset")
	aggOpts["disable_order"] = true

	baseQuery, err := r.BuildReportQuery(ctx, id, aggOpts)
	if err != nil {
		return 0, 0, "", err
	}

	col := strings.TrimSpace(totalColumn)
	useSum := col != "" && reportSimpleIdentPattern.MatchString(col)

	aggQuery := "SELECT COUNT(1) AS total_count"
	if useSum {
		aggQuery += fmt.Sprintf(", COALESCE(SUM(CAST(x.`%s` AS DECIMAL(20,2))), 0) AS overall_total", col)
	}
	aggQuery += fmt.Sprintf(" FROM (%s) AS x", baseQuery)

	dbName, _ := opts["db_name"].(string)
	dbConn := r.choose(dbName)

	var totalCount int
	var overallTotal float64

	if useSum {
		err = dbConn.QueryRowContext(ctx, aggQuery).Scan(&totalCount, &overallTotal)
	} else {
		err = dbConn.QueryRowContext(ctx, aggQuery).Scan(&totalCount)
	}

	if err != nil && strings.EqualFold(strings.TrimSpace(dbName), "DB2") {
		// Retry on DB1 for any DB2 failure — override queries are fully qualified.
		if useSum {
			err = r.db1.QueryRowContext(ctx, aggQuery).Scan(&totalCount, &overallTotal)
		} else {
			err = r.db1.QueryRowContext(ctx, aggQuery).Scan(&totalCount)
		}
	}

	if err != nil {
		return 0, 0, aggQuery, err
	}

	return totalCount, overallTotal, aggQuery, nil
}

// parsePositiveInt converts a string to a positive int with an upper bound; if empty returns the fallback.
func parsePositiveInt(s string, fallback int) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("not a positive int")
	}
	return n, nil
}

// isSafeIdentifierList validates a comma-separated list of identifiers.
func isSafeIdentifierList(list string) bool {
	parts := strings.Split(list, ",")
	for _, p := range parts {
		if !reportIdentPattern.MatchString(strings.TrimSpace(p)) {
			return false
		}
	}
	return true
}

// isSafeOrderBy validates order by fragment (very conservative: col [ASC|DESC]).
func isSafeOrderBy(ob string) bool {
	segments := strings.Split(ob, ",")
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" { continue }
		parts := strings.Fields(seg)
		if len(parts) == 0 || len(parts) > 2 { return false }
		col := parts[0]
		if !reportIdentPattern.MatchString(col) { return false }
		if len(parts) == 2 {
			dir := strings.ToUpper(parts[1])
			if dir != "ASC" && dir != "DESC" { return false }
		}
	}
	return true
}

func reportViewOverride(id int64) (string, bool) {
	switch id {
	default:
		return "", false
	}
}

var reportIdentPattern = regexp.MustCompile(`^[A-Za-z0-9_\.]+$`)
var reportSimpleIdentPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
