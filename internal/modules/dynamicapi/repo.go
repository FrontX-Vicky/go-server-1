package dynamicapi

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"server_1/internal/core/db"
)

type Repo struct {
	db *sql.DB
}

func NewRepo() *Repo {
	return &Repo{
		db: db.DB("DB1"),
	}
}

type QueryResult struct {
	Rows  []map[string]any
	Total *int64
}

func (r *Repo) Fetch(ctx context.Context, req *QueryRequest) (*QueryResult, error) {
	selectClause, err := buildSelectClause(req.Select, req.Distinct)
	if err != nil {
		return nil, err
	}
	table, err := quoteQualified(req.Table)
	if err != nil {
		return nil, err
	}

	whereClause, whereArgs, err := buildWhereClause(req.Filters)
	if err != nil {
		return nil, err
	}

	searchClause, searchArgs, err := buildSearchClause(req.Search)
	if err != nil {
		return nil, err
	}

	allWhere := []string{}
	args := []any{}
	if whereClause != "" {
		allWhere = append(allWhere, whereClause)
		args = append(args, whereArgs...)
	}
	if searchClause != "" {
		allWhere = append(allWhere, searchClause)
		args = append(args, searchArgs...)
	}

	groupByClause, err := buildGroupByClause(req.GroupBy)
	if err != nil {
		return nil, err
	}

	havingClause, havingArgs, err := buildWhereClause(req.Having)
	if err != nil {
		return nil, err
	}
	if havingClause != "" {
		args = append(args, havingArgs...)
	}

	orderClause, err := buildOrderClause(req.Sort)
	if err != nil {
		return nil, err
	}

	baseSQL := fmt.Sprintf("FROM %s", table)
	if len(allWhere) > 0 {
		baseSQL += " WHERE " + strings.Join(allWhere, " AND ")
	}
	if groupByClause != "" {
		baseSQL += " GROUP BY " + groupByClause
	}
	if havingClause != "" {
		baseSQL += " HAVING " + havingClause
	}

	dataSQL := fmt.Sprintf("SELECT %s %s", selectClause, baseSQL)
	if orderClause != "" {
		dataSQL += " ORDER BY " + orderClause
	}

	limit := req.Page.Size
	offset := (req.Page.Number - 1) * req.Page.Size
	dataSQL += " LIMIT ? OFFSET ?"
	dataArgs := append([]any{}, args...)
	dataArgs = append(dataArgs, limit, offset)

	rows, err := r.db.QueryContext(ctx, dataSQL, dataArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	resultRows := make([]map[string]any, 0)
	for rows.Next() {
		scanTargets := make([]any, len(columns))
		columnPointers := make([]any, len(columns))
		for i := range scanTargets {
			columnPointers[i] = &scanTargets[i]
		}
		if err := rows.Scan(columnPointers...); err != nil {
			return nil, err
		}
		rowMap := make(map[string]any, len(columns))
		for i, col := range columns {
			rowMap[col] = derefSQLValue(scanTargets[i])
		}
		resultRows = append(resultRows, rowMap)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var total *int64
	if req.IncludeTotal {
		countSQL, countArgs := buildCountQuery(req, selectClause, baseSQL, args)
		var count int64
		if err := r.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&count); err != nil {
			return nil, err
		}
		total = &count
	}

	return &QueryResult{Rows: resultRows, Total: total}, nil
}

func derefSQLValue(v any) any {
	switch val := v.(type) {
	case nil:
		return nil
	case []byte:
		return string(val)
	default:
		return val
	}
}

func buildCountQuery(req *QueryRequest, selectClause, baseSQL string, args []any) (string, []any) {
	hasGrouping := len(req.GroupBy) > 0
	if req.Distinct || hasGrouping {
		inner := fmt.Sprintf("SELECT %s %s", selectClause, baseSQL)
		return fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS sub", inner), args
	}
	return fmt.Sprintf("SELECT COUNT(*) %s", baseSQL), args
}
