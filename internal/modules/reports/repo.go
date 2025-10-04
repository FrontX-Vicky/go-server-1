package reports

import (
    "context"
    "database/sql"
    "fmt"
    "strings"

    "server_1/internal/core/db"
)

type Repo struct {
    db1 *sql.DB
    db2 *sql.DB
}

func NewRepo() *Repo {
    return &Repo{
        db1: db.DB("DB1"),
        db2: db.DB("DB2"),
    }
}

// choose picks a DB by name; mirrors test_items behavior
func (r *Repo) choose(name string) *sql.DB {
    if name == "DB2" { return r.db2 }
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
            rid            sql.NullInt64
            moduleID       sql.NullInt64
            dateFilterCol  sql.NullString
            reportOption   sql.NullString
            dynamicReport  sql.NullBool
            tableName      sql.NullString
            icon           sql.NullString
            showSR         sql.NullBool
            title          sql.NullString
            subtitle       sql.NullString
            total          sql.NullString
            columnName     sql.NullString
            header         sql.NullString
            isAdmin        sql.NullBool
            position       sql.NullInt64
            mpos           sql.NullInt64
            url            sql.NullString
            concatID       sql.NullString
            onclick        sql.NullString
            modal          sql.NullString
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

// BuildReportQuery constructs the SQL query string for a report id using optional
// startDate, endDate and value placeholders (used when dynamic_report is true).
// It returns the final SQL string (without executing it).
func (r *Repo) BuildReportQuery(ctx context.Context, id int64, startDate, endDate, value string) (string, error) {
    meta, err := r.Get(ctx, id)
    if err != nil {
        return "", err
    }

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
        // fetch view definition
        var view string
        err := r.db1.QueryRowContext(ctx, `SELECT view FROM report_view_table WHERE r_id = ?`, id).Scan(&view)
        if err != nil {
            return "", fmt.Errorf("fetch view: %w", err)
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
        // basic escaping for single quotes
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
            // fallback to direct operator
            conds = append(conds, fmt.Sprintf("%s %s '%s'", name, operator, esc))
        }
    }
    if err := rows.Err(); err != nil {
        return "", fmt.Errorf("filters rows: %w", err)
    }

    if len(conds) > 0 {
        query = query + " WHERE " + strings.Join(conds, " AND ")
    }

    return query, nil
}
