package dynamicapi

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func quoteIdentifier(name string) (string, error) {
	if name == "*" {
		return "*", nil
	}
	if !identifierPattern.MatchString(name) {
		return "", fmt.Errorf("invalid identifier %q", name)
	}
	return fmt.Sprintf("`%s`", name), nil
}

func quoteQualified(name string) (string, error) {
	if name == "*" {
		return "*", nil
	}
	parts := strings.Split(name, ".")
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid identifier %q", name)
	}
	quoted := make([]string, len(parts))
	for i, part := range parts {
		q, err := quoteIdentifier(part)
		if err != nil {
			return "", err
		}
		quoted[i] = q
	}
	return strings.Join(quoted, "."), nil
}

func buildSelectClause(cols []string, distinct bool) (string, error) {
	if len(cols) == 0 {
		if distinct {
			return "", errors.New("distinct queries require select fields")
		}
		return "*", nil
	}

	quoted := make([]string, len(cols))
	for i, col := range cols {
		q, err := quoteQualified(col)
		if err != nil {
			return "", err
		}
		quoted[i] = q
	}

	selectExpr := strings.Join(quoted, ", ")
	if distinct {
		return "DISTINCT " + selectExpr, nil
	}
	return selectExpr, nil
}

func buildGroupByClause(cols []string) (string, error) {
	if len(cols) == 0 {
		return "", nil
	}
	quoted := make([]string, len(cols))
	for i, col := range cols {
		q, err := quoteQualified(col)
		if err != nil {
			return "", err
		}
		quoted[i] = q
	}
	return strings.Join(quoted, ", "), nil
}

func buildOrderClause(sorts []SortSpec) (string, error) {
	if len(sorts) == 0 {
		return "", nil
	}
	parts := make([]string, 0, len(sorts)*2)
	for _, sort := range sorts {
		if sort.Field == "" {
			continue
		}
		col, err := quoteQualified(sort.Field)
		if err != nil {
			return "", err
		}
		dir := strings.ToUpper(sort.Dir)
		if dir != "DESC" {
			dir = "ASC"
		}

		switch sort.Nulls {
		case "first":
			parts = append(parts,
				fmt.Sprintf("CASE WHEN %s IS NULL THEN 0 ELSE 1 END ASC", col),
				fmt.Sprintf("%s %s", col, dir),
			)
		case "last":
			parts = append(parts,
				fmt.Sprintf("CASE WHEN %s IS NULL THEN 1 ELSE 0 END ASC", col),
				fmt.Sprintf("%s %s", col, dir),
			)
		default:
			parts = append(parts, fmt.Sprintf("%s %s", col, dir))
		}
	}
	return strings.Join(parts, ", "), nil
}

func buildSearchClause(search *SearchSpec) (string, []any, error) {
	if search == nil || search.Term == "" || len(search.Fields) == 0 {
		return "", nil, nil
	}
	pattern := "%" + strings.ToLower(search.Term) + "%"
	clauses := make([]string, 0, len(search.Fields))
	args := make([]any, 0, len(search.Fields))
	for _, field := range search.Fields {
		col, err := quoteQualified(field)
		if err != nil {
			return "", nil, err
		}
		clauses = append(clauses, fmt.Sprintf("LOWER(%s) LIKE ?", col))
		args = append(args, pattern)
	}
	return "(" + strings.Join(clauses, " OR ") + ")", args, nil
}

func buildWhereClause(group *FilterGroup) (string, []any, error) {
	if group == nil {
		return "", nil, nil
	}
	group.normalize()

	var clauses []string
	var args []any

	for _, expr := range group.Conditions {
		switch {
		case expr.Group != nil:
			clause, clauseArgs, err := buildWhereClause(expr.Group)
			if err != nil {
				return "", nil, err
			}
			if clause == "" {
				continue
			}
			clauses = append(clauses, "("+clause+")")
			args = append(args, clauseArgs...)
		case expr.Condition != nil:
			clause, condArgs, err := buildCondition(expr.Condition)
			if err != nil {
				return "", nil, err
			}
			clauses = append(clauses, clause)
			args = append(args, condArgs...)
		default:
			return "", nil, errors.New("empty filter expression")
		}
	}

	if len(clauses) == 0 {
		return "", args, nil
	}
	joiner := " AND "
	if group.Logic == "or" {
		joiner = " OR "
	}
	return strings.Join(clauses, joiner), args, nil
}

func buildCondition(cond *FilterCondition) (string, []any, error) {
	if cond.Field == "" {
		return "", nil, errors.New("filter condition missing field")
	}
	if cond.Op == "" {
		return "", nil, fmt.Errorf("filter %s missing operator", cond.Field)
	}

	col, err := quoteQualified(cond.Field)
	if err != nil {
		return "", nil, err
	}

	op := strings.ToLower(cond.Op)
	var args []any

	toLower := cond.CaseSensitive != nil && !*cond.CaseSensitive
	colExpr := col
	if toLower && requiresLower(op) {
		colExpr = fmt.Sprintf("LOWER(%s)", col)
	}

	switch op {
	case "eq", "=":
		val, err := normalizeValue(cond.Value, cond.Timezone, toLower)
		if err != nil {
			return "", nil, err
		}
		args = append(args, val)
		return fmt.Sprintf("%s = ?", colExpr), args, nil
	case "neq", "!=":
		val, err := normalizeValue(cond.Value, cond.Timezone, toLower)
		if err != nil {
			return "", nil, err
		}
		args = append(args, val)
		return fmt.Sprintf("%s <> ?", colExpr), args, nil
	case "gt", ">":
		val, err := normalizeValue(cond.Value, cond.Timezone, toLower)
		if err != nil {
			return "", nil, err
		}
		args = append(args, val)
		return fmt.Sprintf("%s > ?", colExpr), args, nil
	case "gte", ">=":
		val, err := normalizeValue(cond.Value, cond.Timezone, toLower)
		if err != nil {
			return "", nil, err
		}
		args = append(args, val)
		return fmt.Sprintf("%s >= ?", colExpr), args, nil
	case "lt", "<":
		val, err := normalizeValue(cond.Value, cond.Timezone, toLower)
		if err != nil {
			return "", nil, err
		}
		args = append(args, val)
		return fmt.Sprintf("%s < ?", colExpr), args, nil
	case "lte", "<=":
		val, err := normalizeValue(cond.Value, cond.Timezone, toLower)
		if err != nil {
			return "", nil, err
		}
		args = append(args, val)
		return fmt.Sprintf("%s <= ?", colExpr), args, nil
	case "in":
		values, err := normalizeArray(cond.Value, cond.Timezone, toLower)
		if err != nil {
			return "", nil, err
		}
		if len(values) == 0 {
			return "", nil, fmt.Errorf("operator IN requires values for %s", cond.Field)
		}
		args = append(args, values...)
		placeholders := strings.Repeat("?,", len(values))
		return fmt.Sprintf("%s IN (%s)", colExpr, strings.TrimRight(placeholders, ",")), args, nil
	case "notin":
		values, err := normalizeArray(cond.Value, cond.Timezone, toLower)
		if err != nil {
			return "", nil, err
		}
		if len(values) == 0 {
			return "", nil, fmt.Errorf("operator NOT IN requires values for %s", cond.Field)
		}
		args = append(args, values...)
		placeholders := strings.Repeat("?,", len(values))
		return fmt.Sprintf("%s NOT IN (%s)", colExpr, strings.TrimRight(placeholders, ",")), args, nil
	case "between":
		values, err := normalizeArray(cond.Value, cond.Timezone, toLower)
		if err != nil {
			return "", nil, err
		}
		if len(values) != 2 {
			return "", nil, fmt.Errorf("operator BETWEEN expects exactly 2 values for %s", cond.Field)
		}
		args = append(args, values...)
		return fmt.Sprintf("%s BETWEEN ? AND ?", colExpr), args, nil
	case "contains":
		str, err := normalizeString(cond.Value, cond.Timezone, toLower)
		if err != nil {
			return "", nil, err
		}
		args = append(args, "%"+str+"%")
		return fmt.Sprintf("%s LIKE ?", colExpr), args, nil
	case "startswith":
		str, err := normalizeString(cond.Value, cond.Timezone, toLower)
		if err != nil {
			return "", nil, err
		}
		args = append(args, str+"%")
		return fmt.Sprintf("%s LIKE ?", colExpr), args, nil
	case "endswith":
		str, err := normalizeString(cond.Value, cond.Timezone, toLower)
		if err != nil {
			return "", nil, err
		}
		args = append(args, "%"+str)
		return fmt.Sprintf("%s LIKE ?", colExpr), args, nil
	case "isnull":
		return fmt.Sprintf("%s IS NULL", col), nil, nil
	case "notnull":
		return fmt.Sprintf("%s IS NOT NULL", col), nil, nil
	case "like":
		str, err := normalizeString(cond.Value, cond.Timezone, false)
		if err != nil {
			return "", nil, err
		}
		args = append(args, str)
		return fmt.Sprintf("%s LIKE ?", col), args, nil
	default:
		return "", nil, fmt.Errorf("unsupported operator %q for field %s", cond.Op, cond.Field)
	}
}

func requiresLower(op string) bool {
	switch op {
	case "contains", "endswith", "startswith", "eq", "neq", "like", "in", "notin":
		return true
	default:
		return false
	}
}

func normalizeValue(value any, tz string, toLower bool) (any, error) {
	if tz != "" {
		return convertTimeValue(value, tz)
	}
	if toLower {
		if s, ok := value.(string); ok {
			return strings.ToLower(s), nil
		}
	}
	return value, nil
}

func normalizeArray(value any, tz string, toLower bool) ([]any, error) {
	raw, ok := value.([]any)
	if !ok {
		if tmp, ok := value.([]interface{}); ok {
			raw = tmp
		} else {
			return nil, errors.New("array value expected")
		}
	}
	out := make([]any, len(raw))
	for i, v := range raw {
		nv, err := normalizeValue(v, tz, toLower)
		if err != nil {
			return nil, err
		}
		out[i] = nv
	}
	return out, nil
}

func normalizeString(value any, _ string, toLower bool) (string, error) {
	str, ok := value.(string)
	if !ok {
		return "", errors.New("string value expected")
	}
	if toLower {
		str = strings.ToLower(str)
	}
	return str, nil
}

func convertTimeValue(value any, tz string) (any, error) {
	str, ok := value.(string)
	if !ok {
		return value, nil
	}
	ts, err := parseInTimezone(str, tz)
	if err != nil {
		return nil, err
	}
	return ts, nil
}

func parseInTimezone(value string, tz string) (time.Time, error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timezone %q: %w", tz, err)
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if ts, err := time.ParseInLocation(layout, value, loc); err == nil {
			return ts.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("failed to parse %q in timezone %s", value, tz)
}
