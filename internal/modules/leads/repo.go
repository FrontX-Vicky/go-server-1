package leads

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"server_1/internal/core/db"
)

const baseQuery = `
SELECT
	*
FROM inquiry_structured_report_view`

var (
	allowedOperators = map[string]struct{}{
		"=":    {},
		"!=":   {},
		">":    {},
		">=":   {},
		"<":    {},
		"<=":   {},
		"LIKE": {},
	}
	columnPattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
)

type Repo struct {
	db *sql.DB
}

func NewRepo() *Repo {
	return &Repo{
		db: db.DB("DB1"),
	}
}

func (r *Repo) FetchByFilters(ctx context.Context, filters [][]string) ([]map[string]any, error) {
	return r.fetchRows(ctx, filters, false)
}

func (r *Repo) BuildTopSummary(ctx context.Context, filters [][]string) (*TopSummary, error) {
	dr, err := extractDateRange(filters)
	if err != nil {
		return nil, err
	}

	baseFilters := removeDateFilters(filters)

	weekRange := deriveWeekRange(dr)
	prevMonthRange := derivePreviousMonthRange(dr)

	var thisWeekRows []map[string]any
	today := truncateToDate(time.Now().In(weekRange.Start.Location()))
	if today.Month() == dr.End.Month() && today.Year() == dr.End.Year() {
		thisWeekRows, err = r.fetchRows(ctx, applyDateRange(baseFilters, weekRange.Start, weekRange.End), false)
		if err != nil {
			return nil, err
		}
	} else {
		thisWeekRows = nil
	}
	thisMonthRows, err := r.fetchRows(ctx, applyDateRange(baseFilters, dr.Start, dr.End), false)
	if err != nil {
		return nil, err
	}
	prevMonthRows, err := r.fetchRows(ctx, applyDateRange(baseFilters, prevMonthRange.Start, prevMonthRange.End), false)
	if err != nil {
		return nil, err
	}

	weekCounts := computeCounts(thisWeekRows)
	monthCounts := computeCounts(thisMonthRows)
	prevCounts := computeCounts(prevMonthRows)

	log.Printf("leads top summary counts | this_week=%+v this_month=%+v prev_month=%+v", weekCounts, monthCounts, prevCounts)

	rows := buildSummaryRows(weekCounts, monthCounts, prevCounts)

	summary := &TopSummary{
		Rows: rows,
		Metadata: SummaryMetadata{
			ThisWeek: RangeMeta{Start: formatDate(weekRange.Start), End: formatDate(weekRange.End)},
			ThisMonth: RangeMeta{
				Start: formatDate(dr.Start),
				End:   formatDate(dr.End),
			},
			PreviousMonth: RangeMeta{Start: formatDate(prevMonthRange.Start), End: formatDate(prevMonthRange.End)},
		},
	}

	return summary, nil
}

func (r *Repo) fetchRows(ctx context.Context, filters [][]string, limitOne bool) ([]map[string]any, error) {
	if r.db == nil {
		return nil, fmt.Errorf("leads repo: database not initialized")
	}

	query, args, err := buildQuery(filters, limitOne)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetch leads: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("columns: %w", err)
	}

	results := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			if b, ok := values[i].([]byte); ok {
				row[col] = string(b)
				continue
			}
			row[col] = values[i]
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	return results, nil
}

func buildQuery(filters [][]string, limitOne bool) (string, []any, error) {
	query := baseQuery + " WHERE 1=1"
	args := make([]any, 0, len(filters))

	for idx, filter := range filters {
		if len(filter) < 3 {
			return "", nil, fmt.Errorf("filter %d: expected [field, operator, value]", idx)
		}
		field := strings.TrimSpace(filter[0])
		op := strings.TrimSpace(strings.ToUpper(filter[1]))
		value := filter[2]
		trimmedValue := strings.TrimSpace(value)

		if field == "" || op == "" {
			continue // skip empty placeholders
		}
		if !columnPattern.MatchString(field) {
			return "", nil, fmt.Errorf("filter %d: invalid field %q", idx, field)
		}
		if _, ok := allowedOperators[op]; !ok {
			return "", nil, fmt.Errorf("filter %d: operator %q not allowed", idx, op)
		}
		if trimmedValue == "" && op != "=" && op != "!=" {
			// empty value with comparison operators collapses the result set; treat as no-op unless explicit equality check
			continue
		}

		query += fmt.Sprintf(" AND %s %s ?", field, op)
		args = append(args, value)
	}

	log.Printf("leads query | sql=%s args=%v", query, args)
	return query, args, nil
}

type dateRange struct {
	Start time.Time
	End   time.Time
}

func extractDateRange(filters [][]string) (dateRange, error) {
	const key = "doi"
	var (
		start *time.Time
		end   *time.Time
	)
	for _, f := range filters {
		if len(f) < 3 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(f[0]), key) {
			op := strings.TrimSpace(f[1])
			val := strings.TrimSpace(f[2])
			ts, err := parseDate(val)
			if err != nil {
				return dateRange{}, fmt.Errorf("invalid date %q: %w", val, err)
			}
			switch op {
			case ">=", ">", "=>":
				start = &ts
			case "<=", "<", "=<":
				end = &ts
			}
		}
	}
	if start == nil || end == nil {
		return dateRange{}, errors.New("top summary requires doi >= and doi <= filters")
	}
	if end.Before(*start) {
		return dateRange{}, errors.New("doi end date precedes start date")
	}
	return dateRange{Start: truncateToDate(*start), End: truncateToDate(*end)}, nil
}

func removeDateFilters(filters [][]string) [][]string {
	out := make([][]string, 0, len(filters))
	for _, f := range filters {
		if len(f) < 3 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(f[0]), "doi") {
			continue
		}
		cp := make([]string, len(f))
		copy(cp, f)
		out = append(out, cp)
	}
	return out
}

func applyDateRange(filters [][]string, start, end time.Time) [][]string {
	out := make([][]string, 0, len(filters)+2)
	for _, f := range filters {
		cp := make([]string, len(f))
		copy(cp, f)
		out = append(out, cp)
	}
	out = append(out, []string{"doi", ">=", formatDate(start)})
	out = append(out, []string{"doi", "<=", formatDate(end)})
	return out
}

func deriveWeekRange(dr dateRange) dateRange {
	loc := dr.Start.Location()
	if loc == nil {
		loc = time.UTC
	}
	today := truncateToDate(time.Now().In(loc))
	if today.Before(dr.Start) {
		today = truncateToDate(dr.Start)
	}
	if today.After(dr.End) {
		today = truncateToDate(dr.End)
	}

	offset := int(today.Weekday()) // Sunday = 0
	start := time.Date(today.Year(), today.Month(), today.Day()-offset, 0, 0, 0, 0, loc)
	if start.Before(dr.Start) {
		start = truncateToDate(dr.Start)
	}
	end := start.AddDate(0, 0, 6)
	if end.After(dr.End) {
		end = truncateToDate(dr.End)
	}
	return dateRange{Start: truncateToDate(start), End: truncateToDate(end)}
}

func derivePreviousMonthRange(dr dateRange) dateRange {
	loc := dr.Start.Location()
	if loc == nil {
		loc = time.UTC
	}
	currentMonthStart := time.Date(dr.Start.Year(), dr.Start.Month(), 1, 0, 0, 0, 0, loc)
	prevMonthEnd := currentMonthStart.AddDate(0, 0, -1)
	prevMonthStart := time.Date(prevMonthEnd.Year(), prevMonthEnd.Month(), 1, 0, 0, 0, 0, loc)
	return dateRange{Start: prevMonthStart, End: prevMonthEnd}
}

func parseDate(value string) (time.Time, error) {
	layouts := []string{
		"2006-01-02",
		time.RFC3339,
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if ts, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return ts, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse date: %s", value)
}

func truncateToDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func formatDate(t time.Time) string {
	return truncateToDate(t).Format("2006-01-02")
}

type summaryCounts struct {
	TotalLeads            int64
	OrientationBookings   int64
	OrientationAttendance int64
	Enrollments           int64
}

func computeCounts(rows []map[string]any) summaryCounts {
	out := summaryCounts{
		TotalLeads: int64(len(rows)),
	}
	for _, row := range rows {
		if isTruthy(row["demo_registered"]) || isTruthy(row["demo_registered_string"]) || toInt64(row["demo_registered_count"]) > 0 {
			out.OrientationBookings++
		}
		if isTruthy(row["demo_attended"]) || isTruthy(row["demo_attended_string"]) || toInt64(row["demo_attendance_count"]) > 0 {
			out.OrientationAttendance++
		}
		if isTruthy(row["inquiry_converted"]) || isTruthy(row["inquiry_converted_string"]) {
			out.Enrollments++
		}
	}
	return out
}

type TopSummary struct {
	Rows     []SummaryRow    `json:"rows"`
	Metadata SummaryMetadata `json:"metadata"`
}

type SummaryRow struct {
	Metric           string      `json:"metric"`
	ThisWeek         SummaryCell `json:"this_week"`
	ThisMonth        SummaryCell `json:"this_month"`
	PreviousMonth    SummaryCell `json:"previous_month"`
	PercentChangeMoM SummaryCell `json:"percent_change_mom"`
	Target           SummaryCell `json:"target"`
}

type SummaryCell struct {
	Value   float64 `json:"value"`
	Display string  `json:"display"`
	Trend   string  `json:"trend,omitempty"`
}

type SummaryMetadata struct {
	ThisWeek      RangeMeta `json:"this_week"`
	ThisMonth     RangeMeta `json:"this_month"`
	PreviousMonth RangeMeta `json:"previous_month"`
}

type RangeMeta struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

func buildSummaryRows(week, month, prev summaryCounts) []SummaryRow {
	rows := []SummaryRow{
		buildCountRow("Total Leads Generated", week.TotalLeads, month.TotalLeads, prev.TotalLeads, 3500),
		buildCountRow("Orientation Bookings", week.OrientationBookings, month.OrientationBookings, prev.OrientationBookings, 2500),
		buildCountRow("Orientation Attendance", week.OrientationAttendance, month.OrientationAttendance, prev.OrientationAttendance, 1800),
		buildCountRow("Enrollments", week.Enrollments, month.Enrollments, prev.Enrollments, 700),
		buildPercentRow("Orientation → Enrollment %", rate(week.Enrollments, week.OrientationBookings), rate(month.Enrollments, month.OrientationBookings), rate(prev.Enrollments, prev.OrientationBookings), 0.45),
		buildUnavailableRow("Cost Per Lead (CPL)", "₹150"),
		buildUnavailableRow("Cost Per Enrollment (CPE)", "₹2,500"),
		buildUnavailableRow("ROAS", "3.5x"),
	}
	return rows
}

func buildCountRow(metric string, week, month, prev int64, target float64) SummaryRow {
	return SummaryRow{
		Metric: metric,
		ThisWeek: SummaryCell{
			Value:   float64(week),
			Display: formatInt(week),
		},
		ThisMonth: SummaryCell{
			Value:   float64(month),
			Display: formatInt(month),
		},
		PreviousMonth: SummaryCell{
			Value:   float64(prev),
			Display: formatInt(prev),
		},
		PercentChangeMoM: percentageCell(percentChange(float64(month), float64(prev))),
		Target: SummaryCell{
			Value:   target,
			Display: formatTarget(metric, target),
		},
	}
}

func buildPercentRow(metric string, week, month, prev, target float64) SummaryRow {
	return SummaryRow{
		Metric: metric,
		ThisWeek: SummaryCell{
			Value:   week,
			Display: formatPercent(week),
		},
		ThisMonth: SummaryCell{
			Value:   month,
			Display: formatPercent(month),
		},
		PreviousMonth: SummaryCell{
			Value:   prev,
			Display: formatPercent(prev),
		},
		PercentChangeMoM: percentageCell(percentChange(month, prev)),
		Target: SummaryCell{
			Value:   target,
			Display: formatPercent(target),
		},
	}
}

func buildUnavailableRow(metric, targetDisplay string) SummaryRow {
	return SummaryRow{
		Metric: metric,
		ThisWeek: SummaryCell{
			Display: "N/A",
		},
		ThisMonth: SummaryCell{
			Display: "N/A",
		},
		PreviousMonth: SummaryCell{
			Display: "N/A",
		},
		PercentChangeMoM: SummaryCell{
			Display: "N/A",
		},
		Target: SummaryCell{
			Display: targetDisplay,
		},
	}
}

func percentageCell(val float64) SummaryCell {
	trend := trendFromPercent(val)
	return SummaryCell{
		Value:   val,
		Display: formatPercentChange(val),
		Trend:   trend,
	}
}

func rate(numerator, denominator int64) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func percentChange(curr, prev float64) float64 {
	if prev == 0 {
		if curr == 0 {
			return 0
		}
		return 100
	}
	return ((curr - prev) / prev) * 100
}

func formatInt(v int64) string {
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	s := strconv.FormatInt(v, 10)
	if len(s) <= 3 {
		return sign + s
	}
	var b strings.Builder
	b.WriteString(sign)
	pre := len(s) % 3
	if pre == 0 {
		pre = 3
	}
	b.WriteString(s[:pre])
	for i := pre; i < len(s); i += 3 {
		b.WriteString(",")
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

func formatPercent(val float64) string {
	return fmt.Sprintf("%.1f%%", val*100)
}

func formatPercentChange(val float64) string {
	if val > 0.0001 {
		return fmt.Sprintf("↑ %.1f%%", val)
	}
	if val < -0.0001 {
		return fmt.Sprintf("↓ %.1f%%", -val)
	}
	return "→ 0.0%"
}

func trendFromPercent(val float64) string {
	switch {
	case val > 0.0001:
		return "up"
	case val < -0.0001:
		return "down"
	default:
		return "flat"
	}
}

func formatTarget(metric string, target float64) string {
	switch metric {
	case "Orientation → Enrollment %":
		return formatPercent(target)
	case "Cost Per Lead (CPL)", "Cost Per Enrollment (CPE)":
		return fmt.Sprintf("₹%s", formatInt(int64(target)))
	default:
		return formatInt(int64(target))
	}
}

func isTruthy(val any) bool {
	switch v := val.(type) {
	case nil:
		return false
	case bool:
		return v
	case int, int8, int16, int32, int64:
		return toInt64(v) != 0
	case uint, uint8, uint16, uint32, uint64:
		return toInt64(v) != 0
	case float32:
		return v != 0
	case float64:
		return v != 0
	case string:
		return truthyString(v)
	case []byte:
		return truthyString(string(v))
	default:
		return truthyString(fmt.Sprint(v))
	}
}

func truthyString(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return false
	}
	switch s {
	case "0", "false", "no", "n", "null", "nil", "none":
		return false
	}
	return true
}

func toInt64(val any) int64 {
	switch v := val.(type) {
	case int:
		return int64(v)
	case int8:
		return int64(v)
	case int16:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case uint:
		return int64(v)
	case uint8:
		return int64(v)
	case uint16:
		return int64(v)
	case uint32:
		return int64(v)
	case uint64:
		if v > ^uint64(0)>>1 {
			return int64(^uint64(0) >> 1)
		}
		return int64(v)
	case float32:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		if s, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return int64(s)
		}
	case []byte:
		if s, err := strconv.ParseFloat(strings.TrimSpace(string(v)), 64); err == nil {
			return int64(s)
		}
	default:
		if s, err := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(v)), 64); err == nil {
			return int64(s)
		}
	}
	return 0
}
