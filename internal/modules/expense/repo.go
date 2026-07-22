package expense

import (
	"context"
	"fmt"
	"strings"

	"server_1/internal/core/db"
)

// Repo provides access to the expense_view.
type Repo struct {
	db1 *db.SQL
}

func NewRepo() *Repo {
	return &Repo{
		db1: db.DBx("DB1"),
	}
}

// BuildQuery returns the raw SQL that would be executed for the given request.
func (r *Repo) BuildQuery(ctx context.Context, req ExpenseListRequest) (string, error) {
	query, _, err := r.buildQueryParts(req)
	return query, err
}

// buildQueryParts constructs the SELECT query and count query for expense_view.
func (r *Repo) buildQueryParts(req ExpenseListRequest) (string, string, error) {
	var conds []string

	if req.StartDate != "" {
		conds = append(conds, fmt.Sprintf("date >= '%s'", sanitize(req.StartDate)))
	}
	if req.EndDate != "" {
		conds = append(conds, fmt.Sprintf("date <= '%s'", sanitize(req.EndDate)))
	}
	if req.Search != "" {
		s := sanitize(req.Search)
		conds = append(conds, fmt.Sprintf(
			"(reason LIKE '%%%s%%' OR employee LIKE '%%%s%%' OR category LIKE '%%%s%%' OR comment LIKE '%%%s%%')",
			s, s, s, s,
		))
	}

	base := "SELECT * FROM expense_view"
	if len(conds) > 0 {
		base += " WHERE " + strings.Join(conds, " AND ")
	}

	countQuery := "SELECT COUNT(1) FROM (" + base + ") AS cnt_q"

	limit := req.Limit
	if limit <= 0 {
		limit = 25
	}
	if limit > 500 {
		limit = 500
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	dataQuery := fmt.Sprintf("%s LIMIT %d OFFSET %d", base, limit, offset)
	return dataQuery, countQuery, nil
}

// GetExpenseList queries expense_view and returns paginated rows.
func (r *Repo) GetExpenseList(ctx context.Context, req ExpenseListRequest) (*ExpenseListResponse, string, error) {
	dataQuery, countQuery, err := r.buildQueryParts(req)
	if err != nil {
		return nil, "", err
	}

	// ── Count ──────────────────────────────────────────────────────────────
	var totalCount int
	if err := r.db1.QueryRowContext(ctx, countQuery).Scan(&totalCount); err != nil {
		return nil, dataQuery, fmt.Errorf("count expense_view: %w", err)
	}

	// ── Data ───────────────────────────────────────────────────────────────
	rows, err := r.db1.QueryContext(ctx, dataQuery)
	if err != nil {
		return nil, dataQuery, fmt.Errorf("query expense_view: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, dataQuery, err
	}

	result := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, dataQuery, err
		}
		rowMap := map[string]any{}
		for i, col := range cols {
			v := vals[i]
			if b, ok := v.([]byte); ok {
				rowMap[col] = string(b)
			} else {
				rowMap[col] = v
			}
		}
		result = append(result, rowMap)
	}
	if err := rows.Err(); err != nil {
		return nil, dataQuery, err
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 25
	}

	pageCount := 0
	if limit > 0 && totalCount > 0 {
		pageCount = (totalCount + limit - 1) / limit
	}

	return &ExpenseListResponse{
		Rows: result,
		Pagination: ExpensePagination{
			Limit:      limit,
			Offset:     req.Offset,
			TotalCount: totalCount,
			PageCount:  pageCount,
		},
	}, dataQuery, nil
}

// sanitize strips single-quotes to prevent SQL injection in plain string interpolation.
func sanitize(s string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "'", "")
}

// FetchOptions fetches particulars and type heads for inline editing.
func (r *Repo) FetchOptions(ctx context.Context) (*ExpenseOptions, error) {
	opts := &ExpenseOptions{
		Particulars: []ParticularOption{},
		TypeHeads:   []TypeHeadOption{},
	}

	// 1. Fetch Particulars (expense_category)
	catQuery := "SELECT id, category FROM expense_category ORDER BY category ASC"
	rows, err := r.db1.QueryContext(ctx, catQuery)
	if err != nil {
		return nil, fmt.Errorf("query particulars: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var p ParticularOption
		if err := rows.Scan(&p.ID, &p.Category); err != nil {
			return nil, fmt.Errorf("scan particular: %w", err)
		}
		opts.Particulars = append(opts.Particulars, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("particulars loop: %w", err)
	}

	// 2. Fetch Type Heads (expense_type_head_master_view)
	thQuery := "SELECT id, reference_code, type_of_expense, type_head1, type_head2, type_head3 FROM expense_type_head_master_view WHERE park = 0 ORDER BY reference_code ASC"
	thRows, err := r.db1.QueryContext(ctx, thQuery)
	if err != nil {
		return nil, fmt.Errorf("query type_heads: %w", err)
	}
	defer thRows.Close()

	for thRows.Next() {
		var t TypeHeadOption
		if err := thRows.Scan(&t.ID, &t.ReferenceCode, &t.TypeOfExpense, &t.TypeHead1, &t.TypeHead2, &t.TypeHead3); err != nil {
			return nil, fmt.Errorf("scan type_head: %w", err)
		}
		opts.TypeHeads = append(opts.TypeHeads, t)
	}
	if err := thRows.Err(); err != nil {
		return nil, fmt.Errorf("type_heads loop: %w", err)
	}

	return opts, nil
}

// UpdateExpenseInline updates the editable fields of an expense record.
func (r *Repo) UpdateExpenseInline(ctx context.Context, id int, req UpdateExpenseInlineRequest) error {
	query := `
		UPDATE expense 
		SET ecat_id = ?, distribute_in = ?, type_head = ?, type_head1 = ?, type_head2 = ?, type_head3 = ?
		WHERE id = ? AND park = 0
	`
	res, err := r.db1.ExecContext(ctx, query, req.EcatID, req.DistributeIn, req.TypeHead, req.TypeHead1, req.TypeHead2, req.TypeHead3, id)
	if err != nil {
		return fmt.Errorf("update expense: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check affected rows: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("expense record %d not found or already parked", id)
	}
	return nil
}
