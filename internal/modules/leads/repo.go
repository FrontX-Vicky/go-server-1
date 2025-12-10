package leads

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"server_1/internal/core/db"
)

const baseQuery = `
SELECT
	*
FROM inquiry_structured_report_view`

var metaSourceIDs = []string{"22", "23"}

// var metaSourceNames = map[string]struct{}{
// 	"Instagram Ad Form": {},
// 	"Facebook Ad Form":  {},
// }

const metaGroupName = "Meta - Lead Form"

var sourceBadges = map[string]string{
	metaGroupName:       "PM",
	"Google Ad Form 1":  "PM",
	"Instagram Ad Form": "PM",
	"Facebook Ad Form":  "PM",
}

const campaignSpendSQL = `SELECT
	c.campaign_name,
	SUM(hfa.spend) AS spend
FROM heard_from_adcost AS hfa
JOIN heard_from AS hf
  ON hf.ad_id = hfa.ad_id
JOIN campaign AS c
  ON c.campaign_id = hf.campaign_id
WHERE
  hfa.date >= ?
  AND hfa.date < ?
GROUP BY
  c.campaign_name
HAVING
  SUM(hfa.spend) > 0`

const heardFromSpendSQL = `SELECT
	hf.id,
	COALESCE(SUM(hfc.spend), 2) AS spend
FROM heard_from_adcost AS hfc
INNER JOIN heard_from AS hf
	ON hfc.ad_id = hf.ad_id
WHERE
	hfc.date BETWEEN ? AND ?
	AND hfc.spend > 0
GROUP BY
	hf.id`

const (
	arrowUp   = "\u2191"
	arrowDown = "\u2193"
	arrowFlat = "\u2192"
)

type campaignKey struct {
	Platform string
	Name     string
}

var (
	allowedOperators = map[string]struct{}{
		"=":    {},
		"!=":   {},
		">":    {},
		">=":   {},
		"<":    {},
		"<=":   {},
		"LIKE": {},
		"IN":   {},
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
	metaFilters := withMetaSourceFilter(baseFilters)

	weekRange := deriveWeekRange(dr)
	prevMonthRange := derivePreviousMonthRange(dr)

	var (
		thisWeekRows          []map[string]any
		thisMonthRows         []map[string]any
		prevMonthRows         []map[string]any
		weekSpend, monthSpend float64
		prevMonthSpend        float64
	)

	today := truncateToDate(time.Now().In(weekRange.Start.Location()))
	if today.Month() == dr.End.Month() && today.Year() == dr.End.Year() {
		weekFilters := applyDateRange(metaFilters, weekRange.Start, weekRange.End)
		thisWeekRows, err = r.fetchRows(ctx, weekFilters, false)
		if err != nil {
			return nil, err
		}
		weekSpend, err = r.sumMetaSpend(ctx, weekRange.Start, weekRange.End)
		if err != nil {
			return nil, err
		}
	}

	monthFilters := applyDateRange(metaFilters, dr.Start, dr.End)
	thisMonthRows, err = r.fetchRows(ctx, monthFilters, false)
	if err != nil {
		return nil, err
	}
	monthSpend, err = r.sumMetaSpend(ctx, dr.Start, dr.End)
	if err != nil {
		return nil, err
	}

	prevFilters := applyDateRange(metaFilters, prevMonthRange.Start, prevMonthRange.End)
	prevMonthRows, err = r.fetchRows(ctx, prevFilters, false)
	if err != nil {
		return nil, err
	}
	prevMonthSpend, err = r.sumMetaSpend(ctx, prevMonthRange.Start, prevMonthRange.End)
	if err != nil {
		return nil, err
	}

	weekCounts := computeCounts(thisWeekRows)
	monthCounts := computeCounts(thisMonthRows)
	prevCounts := computeCounts(prevMonthRows)

	log.Printf("leads top summary counts | this_week=%+v this_month=%+v prev_month=%+v", weekCounts, monthCounts, prevCounts)

	rows := buildSummaryRows(weekCounts, monthCounts, prevCounts, weekSpend, monthSpend, prevMonthSpend)

	summary := &TopSummary{
		Rows: rows,
		Metadata: SummaryMetadata{
			ThisWeek: Range{Start: formatDate(weekRange.Start), End: formatDate(weekRange.End)},
			ThisMonth: Range{
				Start: formatDate(dr.Start),
				End:   formatDate(dr.End),
			},
			PreviousMonth: Range{Start: formatDate(prevMonthRange.Start), End: formatDate(prevMonthRange.End)},
		},
	}

	return summary, nil
}

func (r *Repo) BuildSourceBreakdown(ctx context.Context, filters [][]string) (*SourceBreakdown, error) {
	dr, err := extractDateRange(filters)
	if err != nil {
		return nil, err
	}
	baseFilters := removeDateFilters(filters)
	periodFilters := applyDateRange(baseFilters, dr.Start, dr.End)

	rows, err := r.fetchRows(ctx, periodFilters, false)
	if err != nil {
		return nil, err
	}

	if len(rows) == 0 {
		return &SourceBreakdown{
			Rows:  []SourceRow{},
			Range: Range{Start: formatDate(dr.Start), End: formatDate(dr.End)},
			// Groups: sourceGroupsMetadata(),
		}, nil
	}

	type sourceAggregate struct {
		counts summaryCounts
		spend  float64
	}

	countsBySource := make(map[string]*sourceAggregate)
	sourceByHeardFrom := make(map[int64]string)
	for _, row := range rows {
		// src := groupSourceName(normalizeString(row["primary_source"]))
		src := normalizeString(row["primary_source"])
		agg := countsBySource[src]
		if agg == nil {
			agg = &sourceAggregate{}
			countsBySource[src] = agg
		}
		accumulateCounts(&agg.counts, row)
		if heardFromID := toInt64(row["primary_heard_from_id"]); heardFromID > 0 {
			if _, exists := sourceByHeardFrom[heardFromID]; !exists {
				sourceByHeardFrom[heardFromID] = src
			}
		}
	}

	spendByHeardFrom, err := r.sumSpendByHeardFrom(ctx, dr.Start, dr.End)
	if err != nil {
		return nil, err
	}
	for heardFromID, spend := range spendByHeardFrom {
		source := sourceByHeardFrom[heardFromID]
		if agg := countsBySource[source]; agg != nil {
			agg.spend += spend
		}
	}

	type sourceEntry struct {
		name string
		agg  *sourceAggregate
	}
	entries := make([]sourceEntry, 0, len(countsBySource))
	for name, agg := range countsBySource {
		entries = append(entries, sourceEntry{name: name, agg: agg})
	}

	sort.Slice(entries, func(i, j int) bool {
		badgeI, hasBadgeI := sourceBadges[entries[i].name]
		badgeJ, hasBadgeJ := sourceBadges[entries[j].name]
		if hasBadgeI != hasBadgeJ {
			return hasBadgeI
		}
		if hasBadgeI && hasBadgeJ {
			if badgeI != badgeJ {
				return badgeI < badgeJ
			}
		}
		a := entries[i].agg.counts.TotalLeads
		b := entries[j].agg.counts.TotalLeads
		if a != b {
			return a > b
		}
		return entries[i].name < entries[j].name
	})

	resultRows := make([]SourceRow, 0, len(entries))
	for _, entry := range entries {
		name := entry.name
		agg := entry.agg
		counts := agg.counts
		spend := agg.spend
		row := SourceRow{
			Source:                  name,
			Leads:                   makeCountCell(counts.TotalLeads),
			OrientationBooked:       makeCountCell(counts.OrientationBookings),
			OrientationAttended:     makeCountCell(counts.OrientationAttendance),
			Enrollments:             makeCountCell(counts.Enrollments),
			OrientationToEnrollment: makePercentCell(rate(counts.Enrollments, counts.OrientationAttendance)),
			Spend:                   currencyCell(spend),
			CPL:                     costCell(spend, counts.TotalLeads),
			CPE:                     costCell(spend, counts.Enrollments),
			ROAS:                    roasCell(counts.Revenue, spend),
		}
		if badge, ok := sourceBadges[name]; ok {
			row.SourceBadge = badge
		}
		resultRows = append(resultRows, row)
	}

	return &SourceBreakdown{
		Rows:  resultRows,
		Range: Range{Start: formatDate(dr.Start), End: formatDate(dr.End)},
		// Groups: sourceGroupsMetadata(),
	}, nil
}

func (r *Repo) BuildCenterPerformance(ctx context.Context, filters [][]string) (*CenterPerformance, error) {
	dr, err := extractDateRange(filters)
	if err != nil {
		return nil, err
	}
	baseFilters := removeDateFilters(filters)
	metaFilters := withMetaSourceFilter(baseFilters)
	periodFilters := applyDateRange(metaFilters, dr.Start, dr.End)

	rows, err := r.fetchRows(ctx, periodFilters, false)
	if err != nil {
		return nil, err
	}

	if len(rows) == 0 {
		return &CenterPerformance{
			Rows: []CenterRow{},
			Totals: CenterTotals{
				Leads:             makeCountCell(0),
				OrientationBooked: makeCountCell(0),
				ShowUps:           makeCountCell(0),
				Enrollments:       makeCountCell(0),
			},
			Range: Range{Start: formatDate(dr.Start), End: formatDate(dr.End)},
		}, nil
	}

	type aggregate struct {
		counts summaryCounts
	}

	countsByCity := make(map[string]*aggregate)
	var totalLeads, totalBooked, totalShowUps, totalEnrollments int64
	for _, row := range rows {
		city := normalizeString(row["city"])
		if city == "" {
			city = "Unknown"
		}
		agg := countsByCity[city]
		if agg == nil {
			agg = &aggregate{}
			countsByCity[city] = agg
		}
		accumulateCounts(&agg.counts, row)
	}
	for _, agg := range countsByCity {
		totalLeads += agg.counts.TotalLeads
		totalBooked += agg.counts.OrientationBookings
		totalShowUps += agg.counts.OrientationAttendance
		totalEnrollments += agg.counts.Enrollments
	}

	totalSpend, err := r.sumMetaSpend(ctx, dr.Start, dr.End)
	if err != nil {
		return nil, err
	}

	cities := make([]string, 0, len(countsByCity))
	for city := range countsByCity {
		cities = append(cities, city)
	}
	sort.Slice(cities, func(i, j int) bool {
		a := countsByCity[cities[i]].counts.TotalLeads
		b := countsByCity[cities[j]].counts.TotalLeads
		if a == b {
			return cities[i] < cities[j]
		}
		return a > b
	})

	rowsOut := make([]CenterRow, 0, len(cities))
	for _, city := range cities {
		agg := countsByCity[city]
		counts := agg.counts
		spend := 0.0
		if totalLeads > 0 {
			spend = totalSpend * (float64(counts.TotalLeads) / float64(totalLeads))
		}
		showUpPct := rate(counts.OrientationAttendance, counts.OrientationBookings)
		conversionPct := rate(counts.Enrollments, counts.OrientationAttendance)

		row := CenterRow{
			City:              city,
			Leads:             makeCountCell(counts.TotalLeads),
			OrientationBooked: makeCountCell(counts.OrientationBookings),
			ShowUps:           makeCountCell(counts.OrientationAttendance),
			Enrollments:       makeCountCell(counts.Enrollments),
			ShowUpPercent:     makePercentCell(showUpPct),
			ConversionPercent: makePercentCell(conversionPct),
			CAC:               costCell(spend, counts.Enrollments),
			Revenue:           currencyCell(counts.Revenue),
			ROAS:              roasCell(counts.Revenue, spend),
		}
		rowsOut = append(rowsOut, row)
	}

	return &CenterPerformance{
		Rows: rowsOut,
		Totals: CenterTotals{
			Leads:             makeCountCell(totalLeads),
			OrientationBooked: makeCountCell(totalBooked),
			ShowUps:           makeCountCell(totalShowUps),
			Enrollments:       makeCountCell(totalEnrollments),
		},
		Range: Range{Start: formatDate(dr.Start), End: formatDate(dr.End)},
	}, nil
}

func (r *Repo) BuildFunnelStageTracking(ctx context.Context, filters [][]string) (*FunnelStageTracking, error) {
	dr, err := extractDateRange(filters)
	if err != nil {
		return nil, err
	}

	baseFilters := removeDateFilters(filters)
	metaFilters := withMetaSourceFilter(baseFilters)

	periodFilters := applyDateRange(metaFilters, dr.Start, dr.End)

	rows, err := r.fetchRows(ctx, periodFilters, false)
	if err != nil {
		return nil, err
	}

	counts := computeCounts(rows)
	if counts.TotalLeads == 0 {
		return &FunnelStageTracking{
			Rows: []FunnelRow{
				{Stage: "Lead Generated", Total: makeCountCell(0), PercentPrev: dashCell(), AverageDays: dashCell()},
				{Stage: "Orientation Booked", Total: makeCountCell(0), PercentPrev: dashCell(), AverageDays: dashCell()},
				{Stage: "Orientation Attended", Total: makeCountCell(0), PercentPrev: dashCell(), AverageDays: dashCell()},
				{Stage: "Enrollment Confirmed", Total: makeCountCell(0), PercentPrev: dashCell(), AverageDays: dashCell()},
				{Stage: "Orientation -> Enrollment", Total: makeCountCell(0), PercentPrev: dashCell(), AverageDays: dashCell()},
			},
			Range: Range{Start: formatDate(dr.Start), End: formatDate(dr.End)},
		}, nil
	}

	type durationAgg struct {
		sum   float64
		count int64
	}
	addDuration := func(agg *durationAgg, days float64) {
		if agg == nil || days < 0 || math.IsNaN(days) || math.IsInf(days, 0) {
			return
		}
		agg.sum += days
		agg.count++
	}
	avgDuration := func(agg durationAgg) (float64, bool) {
		if agg.count == 0 {
			return 0, false
		}
		return agg.sum / float64(agg.count), true
	}

	var (
		bookedDur   durationAgg
		attendedDur durationAgg
		enrolledDur durationAgg
	)

	for _, row := range rows {
		leadDate, okLead := getDate(row["doi"])
		bookDate, okBook := getDateFromFields(row,
			"demo_registered_date", "demo_registered_datetime", "demo_registered_at", "demo_registered_ts", "demo_registered_time")
		attendDate, okAttend := getDateConditional(row,
			[]string{"demo_date", "demo_datetime", "demo_attended_at", "demo_attended_date"}, row["demo_attended"])
		enrollDate, okEnroll := getDateConditional(row,
			[]string{"date_of_conversion", "conversion_date", "enrolled_at", "enrollment_date"}, row["inquiry_converted"])

		if okLead && okBook {
			addDuration(&bookedDur, bookDate.Sub(leadDate).Hours()/24)
		}
		if okBook && okAttend {
			addDuration(&attendedDur, attendDate.Sub(bookDate).Hours()/24)
		}
		if okAttend && okEnroll {
			addDuration(&enrolledDur, enrollDate.Sub(attendDate).Hours()/24)
		}
	}

	avgBook, okBookDur := avgDuration(bookedDur)
	avgAttend, okAttendDur := avgDuration(attendedDur)
	avgEnroll, okEnrollDur := avgDuration(enrolledDur)

	rowsOut := []FunnelRow{
		{
			Stage:       "Lead Generated",
			Total:       makeCountCell(counts.TotalLeads),
			PercentPrev: dashCell(),
			AverageDays: dashCell(),
		},
		{
			Stage:       "Orientation Booked",
			Total:       makeCountCell(counts.OrientationBookings),
			PercentPrev: makePercentCell(rate(counts.OrientationBookings, counts.TotalLeads)),
			AverageDays: durationCell(avgBook, okBookDur),
		},
		{
			Stage:       "Orientation Attended",
			Total:       makeCountCell(counts.OrientationAttendance),
			PercentPrev: makePercentCell(rate(counts.OrientationAttendance, counts.OrientationBookings)),
			AverageDays: durationCell(avgAttend, okAttendDur),
		},
		{
			Stage:       "Enrollment Confirmed",
			Total:       makeCountCell(counts.Enrollments),
			PercentPrev: makePercentCell(rate(counts.Enrollments, counts.OrientationAttendance)),
			AverageDays: durationCell(avgEnroll, okEnrollDur),
		},
		{
			Stage:       "Orientation -> Enrollment",
			Total:       makeCountCell(counts.Enrollments),
			PercentPrev: makePercentCell(rate(counts.Enrollments, counts.OrientationAttendance)),
			AverageDays: dashCell(),
		},
	}

	return &FunnelStageTracking{
		Rows:  rowsOut,
		Range: Range{Start: formatDate(dr.Start), End: formatDate(dr.End)},
	}, nil
}

func (r *Repo) BuildCampaignPerformance(ctx context.Context, filters [][]string) (*CampaignPerformance, error) {
	dr, err := extractDateRange(filters)
	if err != nil {
		return nil, err
	}

	baseFilters := removeDateFilters(filters)
	metaFilters := withMetaSourceFilter(baseFilters)

	filterWithDates := applyDateRange(metaFilters, dr.Start, dr.End)
	leadsQuery, leadArgs, err := buildQuery(filterWithDates, false)
	if err != nil {
		return nil, err
	}

	rows, err := r.fetchRows(ctx, filterWithDates, false)
	if err != nil {
		return nil, err
	}

	// Fetch distinct interest labels for UI (optional)
	interestLabels, _ := r.uniqueInterestStrings(ctx)

	spendMap, spendDebug, err := r.sumMetaSpendByCampaign(ctx, dr.Start, dr.End)
	if err != nil {
		return nil, err
	}

	if len(rows) == 0 {
		return &CampaignPerformance{
			Rows: []CampaignRow{},
			Totals: CampaignTotals{
				Leads:                 makeCountCell(0),
				OrientationAttendance: makeCountCell(0),
				Enrollments:           makeCountCell(0),
				Spend:                 currencyCell(0),
				Revenue:               currencyCell(0),
				SQL:                   makeCountCell(0),
				HOT:                   makeCountCell(0),
				WARM:                  makeCountCell(0),
				COLD:                  makeCountCell(0),
			},
			Queries: CampaignQueries{
				Leads: QueryDebug{SQL: leadsQuery, Params: stringifyArgs(leadArgs)},
				Spend: spendDebug,
			},
			Range:          Range{Start: formatDate(dr.Start), End: formatDate(dr.End)},
			InterestLabels: interestLabels,
		}, nil
	}

	type campaignAggregate struct {
		counts       summaryCounts
		revenue      float64
		sqlCount     int64
		hotCount     int64
		warmCount    int64
		coldCount    int64
		coldByReason map[string]int64
	}

	aggregates := make(map[string]*campaignAggregate)
	var totalCounts summaryCounts
	var totalRevenue float64
	var totalSQL, totalHOT, totalWARM, totalCOLD int64
	for _, row := range rows {
		campaign := normalizeString(row["primary_campaign_name"])
		if campaign == "" {
			campaign = "Unknown"
		}

		agg := aggregates[campaign]
		if agg == nil {
			agg = &campaignAggregate{coldByReason: make(map[string]int64)}
			aggregates[campaign] = agg
		}
		accumulateCounts(&agg.counts, row)
		agg.revenue += toFloat(row["payment"])

		// Count SQL flag
		if strings.EqualFold(normalizeString(row["sql_flag"]), "Yes") {
			agg.sqlCount++
			totalSQL++
		}

		// Count interest_string for HOT/WARM, and breakdown reasons for COLD (anything else)
		interestStr := normalizeString(row["interest_string"])
		if strings.EqualFold(interestStr, "Hot") {
			agg.hotCount++
			totalHOT++
		} else if strings.EqualFold(interestStr, "Warm") {
			agg.warmCount++
			totalWARM++
		} else if interestStr != "" {
			// Treat any non-empty, non-Hot/Warm interest string as a Cold reason
			agg.coldCount++
			totalCOLD++
			agg.coldByReason[interestStr] = agg.coldByReason[interestStr] + 1
		}

		accumulateCounts(&totalCounts, row)
		totalRevenue += toFloat(row["payment"])
	}

	campaignNames := make([]string, 0, len(aggregates))
	for name := range aggregates {
		campaignNames = append(campaignNames, name)
	}
	sort.Slice(campaignNames, func(i, j int) bool {
		a := aggregates[campaignNames[i]].counts.TotalLeads
		b := aggregates[campaignNames[j]].counts.TotalLeads
		if a == b {
			return campaignNames[i] < campaignNames[j]
		}
		return a > b
	})

	rowsOut := make([]CampaignRow, 0, len(campaignNames))
	totalSpendAssigned := 0.0
	for _, campaignName := range campaignNames {
		agg := aggregates[campaignName]
		counts := agg.counts
		totalCampaignSpend := spendMap[campaignName]
		spend := totalCampaignSpend
		totalSpendAssigned += spend

		attPercent := rate(counts.OrientationAttendance, counts.OrientationBookings)
		orientEnrollPercent := rate(counts.Enrollments, counts.OrientationAttendance)

		// build cold breakdown slice from map
		var breakdown []CountItem
		if len(agg.coldByReason) > 0 {
			breakdown = make([]CountItem, 0, len(agg.coldByReason))
			for name, cnt := range agg.coldByReason {
				breakdown = append(breakdown, CountItem{Name: name, Count: cnt})
			}
			sort.Slice(breakdown, func(i, j int) bool {
				if breakdown[i].Count == breakdown[j].Count {
					return breakdown[i].Name < breakdown[j].Name
				}
				return breakdown[i].Count > breakdown[j].Count
			})
		}

		rowsOut = append(rowsOut, CampaignRow{
			CampaignName:             campaignName,
			Objective:                "Leads",
			Leads:                    makeCountCell(counts.TotalLeads),
			OrientationAttendance:    makeCountCell(counts.OrientationAttendance),
			Enrollments:              makeCountCell(counts.Enrollments),
			Spend:                    currencyCell(spend),
			CPL:                      costCell(spend, counts.TotalLeads),
			CPE:                      costCell(spend, counts.Enrollments),
			OrientationAttPercent:    makePercentCell(attPercent),
			OrientationEnrollPercent: makePercentCell(orientEnrollPercent),
			Revenue:                  currencyCell(counts.Revenue),
			ROAS:                     roasCell(counts.Revenue, spend),
			SQL:                      makeCountCell(agg.sqlCount),
			HOT:                      makeCountCell(agg.hotCount),
			WARM:                     makeCountCell(agg.warmCount),
			COLD:                     makeCountCell(agg.coldCount),
			ColdBreakdown:            breakdown,
		})
	}

	return &CampaignPerformance{
		Rows: rowsOut,
		Totals: CampaignTotals{
			Leads:                 makeCountCell(totalCounts.TotalLeads),
			OrientationAttendance: makeCountCell(totalCounts.OrientationAttendance),
			Enrollments:           makeCountCell(totalCounts.Enrollments),
			Spend:                 currencyCell(totalSpendAssigned),
			Revenue:               currencyCell(totalRevenue),
			SQL:                   makeCountCell(totalSQL),
			HOT:                   makeCountCell(totalHOT),
			WARM:                  makeCountCell(totalWARM),
			COLD:                  makeCountCell(totalCOLD),
		},
		Queries: CampaignQueries{
			Leads: QueryDebug{SQL: leadsQuery, Params: stringifyArgs(leadArgs)},
			Spend: spendDebug,
		},
		Range:          Range{Start: formatDate(dr.Start), End: formatDate(dr.End)},
		InterestLabels: interestLabels,
	}, nil
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

func (r *Repo) sumMetaSpend(ctx context.Context, start, end time.Time) (float64, error) {
	if r.db == nil {
		return 0, fmt.Errorf("leads repo: database not initialized")
	}
	const q = `SELECT COALESCE(SUM(hfc.spend),2)
				FROM heard_from_adcost hfc
				INNER JOIN heard_from hf ON hfc.ad_id = hf.ad_id
				WHERE hfc.date BETWEEN ? AND ?`

	var total sql.NullFloat64
	if err := r.db.QueryRowContext(ctx, q, formatDate(start), formatDate(end)).Scan(&total); err != nil {
		return 0, fmt.Errorf("meta spend: %w", err)
	}
	if total.Valid {
		return total.Float64, nil
	}
	return 0, nil
}

func (r *Repo) sumSpendByHeardFrom(ctx context.Context, start, end time.Time) (map[int64]float64, error) {
	if r.db == nil {
		return nil, fmt.Errorf("leads repo: database not initialized")
	}
	rows, err := r.db.QueryContext(ctx, heardFromSpendSQL, formatDate(start), formatDate(end))
	if err != nil {
		return nil, fmt.Errorf("heard_from spend: %w", err)
	}
	defer rows.Close()

	result := make(map[int64]float64)
	for rows.Next() {
		var id sql.NullInt64
		var spend sql.NullFloat64
		if err := rows.Scan(&id, &spend); err != nil {
			return nil, fmt.Errorf("heard_from spend scan: %w", err)
		}
		if !id.Valid {
			continue
		}
		if spend.Valid {
			result[id.Int64] = spend.Float64
		} else {
			result[id.Int64] = 0
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("heard_from spend rows: %w", err)
	}
	return result, nil
}
func (r *Repo) sumMetaSpendByCampaign(ctx context.Context, start, end time.Time) (map[string]float64, QueryDebug, error) {
	if r.db == nil {
		return nil, QueryDebug{}, fmt.Errorf("leads repo: database not initialized")
	}
	params := []string{formatDate(start), formatDate(end)}
	rows, err := r.db.QueryContext(ctx, campaignSpendSQL, params[0], params[1])
	if err != nil {
		return nil, QueryDebug{}, fmt.Errorf("meta spend by campaign: %w", err)
	}
	defer rows.Close()

	result := make(map[string]float64)
	for rows.Next() {
		var name sql.NullString
		var spend sql.NullFloat64
		if err := rows.Scan(&name, &spend); err != nil {
			return nil, QueryDebug{}, fmt.Errorf("meta spend by campaign scan: %w", err)
		}
		key := normalizeString(name.String)
		result[key] = spend.Float64
	}
	if err := rows.Err(); err != nil {
		return nil, QueryDebug{}, fmt.Errorf("meta spend by campaign rows: %w", err)
	}
	return result, QueryDebug{SQL: campaignSpendSQL, Params: params}, nil
}

// uniqueInterestStrings returns distinct interest_string values from the inquiry view.
func (r *Repo) uniqueInterestStrings(ctx context.Context) ([]string, error) {
	if r.db == nil {
		return nil, fmt.Errorf("leads repo: database not initialized")
	}
	// Build using baseQuery to keep table source consistent
	const q = `SELECT interest_string FROM (%s) AS v GROUP BY interest_string`
	finalQ := fmt.Sprintf(q, strings.TrimSpace(baseQuery))
	rows, err := r.db.QueryContext(ctx, finalQ)
	if err != nil {
		return nil, fmt.Errorf("distinct interest_string: %w", err)
	}
	defer rows.Close()

	labels := make([]string, 0)
	for rows.Next() {
		var s sql.NullString
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("scan interest_string: %w", err)
		}
		name := normalizeString(s.String)
		if name == "" {
			continue
		}
		labels = append(labels, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows interest_string: %w", err)
	}
	sort.Strings(labels)
	return labels, nil
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
		if trimmedValue == "" && op != "=" && op != "!=" && op != "IN" {
			// empty value with comparison operators collapses the result set; treat as no-op unless explicit equality check
			continue
		}

		if op == "IN" {
			parts := strings.Split(value, ",")
			placeholders := make([]string, 0, len(parts))
			for _, part := range parts {
				val := strings.Trim(strings.TrimSpace(part), "'\"")
				if val == "" {
					continue
				}
				placeholders = append(placeholders, "?")
				args = append(args, val)
			}
			if len(placeholders) == 0 {
				continue
			}
			query += fmt.Sprintf(" AND %s IN (%s)", field, strings.Join(placeholders, ","))
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

func cloneFilters(filters [][]string) [][]string {
	out := make([][]string, 0, len(filters))
	for _, f := range filters {
		cp := make([]string, len(f))
		copy(cp, f)
		out = append(out, cp)
	}
	return out
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

func withMetaSourceFilter(filters [][]string) [][]string {
	out := cloneFilters(filters)
	value := strings.Join(metaSourceIDs, ",")
	out = append(out, []string{"primary_source_id", "IN", value})
	return out
}

func applyDateRange(filters [][]string, start, end time.Time) [][]string {
	out := cloneFilters(filters)
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
	value = strings.TrimSpace(value)
	if value == "" || value == "0000-00-00" || value == "0000-00-00 00:00:00" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	layouts := []string{
		"2006-01-02",
		"2006-01-02 15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05.000000",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05.000",
		"2006-01-02T15:04:05.000000",
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05 -0700 MST",
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
	Revenue               float64
}

func computeCounts(rows []map[string]any) summaryCounts {
	out := summaryCounts{}
	for _, row := range rows {
		accumulateCounts(&out, row)
	}
	return out
}

func accumulateCounts(out *summaryCounts, row map[string]any) {
	if out == nil {
		return
	}
	out.TotalLeads++
	if isTruthy(row["demo_registered"]) || isTruthy(row["demo_registered_string"]) || toInt64(row["demo_registered_count"]) > 0 {
		out.OrientationBookings++
	}
	if isTruthy(row["demo_attended"]) || isTruthy(row["demo_attended_string"]) || toInt64(row["demo_attendance_count"]) > 0 {
		out.OrientationAttendance++
	}
	if isTruthy(row["inquiry_converted"]) || isTruthy(row["inquiry_converted_string"]) {
		out.Enrollments++
	}
	out.Revenue += toFloat(row["payment"])
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
	Badge   string  `json:"badge,omitempty"`
}

type SummaryMetadata struct {
	ThisWeek      Range `json:"this_week"`
	ThisMonth     Range `json:"this_month"`
	PreviousMonth Range `json:"previous_month"`
}

type Range struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type SourceBreakdown struct {
	Rows  []SourceRow `json:"rows"`
	Range Range       `json:"range"`
	// Groups []SourceGroup `json:"groups,omitempty"`
}

type SourceRow struct {
	Source                  string      `json:"source"`
	SourceBadge             string      `json:"source_badge,omitempty"`
	Leads                   SummaryCell `json:"leads"`
	OrientationBooked       SummaryCell `json:"orientation_booked"`
	OrientationAttended     SummaryCell `json:"orientation_attended"`
	Enrollments             SummaryCell `json:"enrollments"`
	OrientationToEnrollment SummaryCell `json:"orientation_to_enrollment"`
	Spend                   SummaryCell `json:"spend"`
	CPL                     SummaryCell `json:"cost_per_lead"`
	CPE                     SummaryCell `json:"cost_per_enrollment"`
	ROAS                    SummaryCell `json:"roas"`
}

// type SourceGroup struct {
// 	Name      string   `json:"name"`
// 	Sources   []string `json:"primary_source"`
// 	SourceIDs []string `json:"primary_source_id,omitempty"`
// }

type CenterPerformance struct {
	Rows   []CenterRow  `json:"rows"`
	Totals CenterTotals `json:"totals"`
	Range  Range        `json:"range"`
}

type CenterRow struct {
	City              string      `json:"city"`
	Leads             SummaryCell `json:"leads"`
	OrientationBooked SummaryCell `json:"orientation_booked"`
	ShowUps           SummaryCell `json:"show_ups"`
	Enrollments       SummaryCell `json:"enrollments"`
	ShowUpPercent     SummaryCell `json:"show_up_percent"`
	ConversionPercent SummaryCell `json:"conversion_percent"`
	CAC               SummaryCell `json:"cac"`
	Revenue           SummaryCell `json:"revenue"`
	ROAS              SummaryCell `json:"roas"`
}

type CenterTotals struct {
	Leads             SummaryCell `json:"leads"`
	OrientationBooked SummaryCell `json:"orientation_booked"`
	ShowUps           SummaryCell `json:"show_ups"`
	Enrollments       SummaryCell `json:"enrollments"`
}

type FunnelStageTracking struct {
	Rows  []FunnelRow `json:"rows"`
	Range Range       `json:"range"`
}

type FunnelRow struct {
	Stage       string      `json:"stage"`
	Total       SummaryCell `json:"total"`
	PercentPrev SummaryCell `json:"percent_prev"`
	AverageDays SummaryCell `json:"avg_days"`
}

type CampaignPerformance struct {
	Rows           []CampaignRow   `json:"rows"`
	Totals         CampaignTotals  `json:"totals"`
	Queries        CampaignQueries `json:"queries"`
	Range          Range           `json:"range"`
	InterestLabels []string        `json:"interest_labels,omitempty"`
}

type CampaignRow struct {
	// Platform                 string      `json:"platform"`
	CampaignName             string      `json:"campaign_name"`
	Objective                string      `json:"objective"`
	Leads                    SummaryCell `json:"leads"`
	OrientationAttendance    SummaryCell `json:"orientation_attendance"`
	Enrollments              SummaryCell `json:"enrollments"`
	Spend                    SummaryCell `json:"spend"`
	CPL                      SummaryCell `json:"cost_per_lead"`
	CPE                      SummaryCell `json:"cost_per_enrollment"`
	OrientationAttPercent    SummaryCell `json:"orientation_att_percent"`
	OrientationEnrollPercent SummaryCell `json:"orientation_enroll_percent"`
	Revenue                  SummaryCell `json:"revenue"`
	ROAS                     SummaryCell `json:"roas"`
	SQL                      SummaryCell `json:"sql"`
	HOT                      SummaryCell `json:"hot"`
	WARM                     SummaryCell `json:"warm"`
	COLD                     SummaryCell `json:"cold"`
	ColdBreakdown            []CountItem `json:"cold_breakdown,omitempty"`
}

// CountItem is a simple label->count pair used for pie chart breakdowns
type CountItem struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type CampaignTotals struct {
	Leads                 SummaryCell `json:"leads"`
	OrientationAttendance SummaryCell `json:"orientation_attendance"`
	Enrollments           SummaryCell `json:"enrollments"`
	Spend                 SummaryCell `json:"spend"`
	Revenue               SummaryCell `json:"revenue"`
	SQL                   SummaryCell `json:"sql"`
	HOT                   SummaryCell `json:"hot"`
	WARM                  SummaryCell `json:"warm"`
	COLD                  SummaryCell `json:"cold"`
}

type CampaignQueries struct {
	Leads QueryDebug `json:"leads"`
	Spend QueryDebug `json:"spend"`
}

type QueryDebug struct {
	SQL    string   `json:"sql"`
	Params []string `json:"params,omitempty"`
}

func buildSummaryRows(week summaryCounts, month summaryCounts, prev summaryCounts, weekSpend, monthSpend, prevSpend float64) []SummaryRow {
	rows := []SummaryRow{
		buildCountRow("Total Leads Generated", week.TotalLeads, month.TotalLeads, prev.TotalLeads, 3500),
		buildCountRow("Orientation Bookings", week.OrientationBookings, month.OrientationBookings, prev.OrientationBookings, 2500),
		buildCountRow("Orientation Attendance", week.OrientationAttendance, month.OrientationAttendance, prev.OrientationAttendance, 1800),
		buildCountRow("Enrollments", week.Enrollments, month.Enrollments, prev.Enrollments, 700),
		buildPercentRow("Orientation -> Enrollment %", rate(week.Enrollments, week.OrientationBookings), rate(month.Enrollments, month.OrientationBookings), rate(prev.Enrollments, prev.OrientationBookings), 0.45),
		buildCostRow("Cost Per Lead (CPL)", weekSpend, week.TotalLeads, monthSpend, month.TotalLeads, prevSpend, prev.TotalLeads, 150),
		buildCostRow("Cost Per Enrollment (CPE)", weekSpend, week.Enrollments, monthSpend, month.Enrollments, prevSpend, prev.Enrollments, 2500),
		buildSpendRow("Total Ad Cost", weekSpend, monthSpend, prevSpend),
		buildRevenueRow("Total Revenue", week.Revenue, month.Revenue, prev.Revenue),
		buildROASRow("ROAS", week.Revenue, weekSpend, month.Revenue, monthSpend, prev.Revenue, prevSpend, 3.5),
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

func buildSpendRow(metric string, weekSpend, monthSpend, prevSpend float64) SummaryRow {
	return SummaryRow{
		Metric: metric,
		ThisWeek: SummaryCell{
			Value:   weekSpend,
			Display: formatCurrency(weekSpend),
		},
		ThisMonth: SummaryCell{
			Value:   monthSpend,
			Display: formatCurrency(monthSpend),
		},
		PreviousMonth: SummaryCell{
			Value:   prevSpend,
			Display: formatCurrency(prevSpend),
		},
		PercentChangeMoM: percentageCell(percentChange(monthSpend, prevSpend)),
		Target: SummaryCell{
			Value:   0,
			Display: "N/A",
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

func buildCostRow(metric string, weekSpend float64, weekDenom int64, monthSpend float64, monthDenom int64, prevSpend float64, prevDenom int64, target float64) SummaryRow {
	weekCell := costCell(weekSpend, weekDenom)
	monthCell := costCell(monthSpend, monthDenom)
	prevCell := costCell(prevSpend, prevDenom)
	return SummaryRow{
		Metric:           metric,
		ThisWeek:         weekCell,
		ThisMonth:        monthCell,
		PreviousMonth:    prevCell,
		PercentChangeMoM: percentageCell(percentChange(monthCell.Value, prevCell.Value)),
		Target: SummaryCell{
			Value:   target,
			Display: formatTarget(metric, target),
		},
	}
}

func buildRevenueRow(metric string, weekRevenue, monthRevenue, prevRevenue float64) SummaryRow {
	return SummaryRow{
		Metric: metric,
		ThisWeek: SummaryCell{
			Value:   weekRevenue,
			Display: formatCurrency(weekRevenue),
		},
		ThisMonth: SummaryCell{
			Value:   monthRevenue,
			Display: formatCurrency(monthRevenue),
		},
		PreviousMonth: SummaryCell{
			Value:   prevRevenue,
			Display: formatCurrency(prevRevenue),
		},
		PercentChangeMoM: percentageCell(percentChange(monthRevenue, prevRevenue)),
		Target: SummaryCell{
			Value:   0,
			Display: "N/A",
		},
	}
}

func buildROASRow(metric string, weekRevenue, weekSpend float64, monthRevenue, monthSpend float64, prevRevenue, prevSpend float64, target float64) SummaryRow {
	weekCell := roasCell(weekRevenue, weekSpend)
	monthCell := roasCell(monthRevenue, monthSpend)
	prevCell := roasCell(prevRevenue, prevSpend)
	return SummaryRow{
		Metric:           metric,
		ThisWeek:         weekCell,
		ThisMonth:        monthCell,
		PreviousMonth:    prevCell,
		PercentChangeMoM: percentageCell(percentChange(monthCell.Value, prevCell.Value)),
		Target: SummaryCell{
			Value:   target,
			Display: formatTarget(metric, target),
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
		return fmt.Sprintf("up %.1f%%", val)
	}
	if val < -0.0001 {
		return fmt.Sprintf("down %.1f%%", -val)
	}
	return "flat 0.0%"
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
	case "Orientation -> Enrollment %":
		return formatPercent(target)
	case "Cost Per Lead (CPL)", "Cost Per Enrollment (CPE)":
		return formatCurrency(target)
	case "ROAS":
		return fmt.Sprintf("%.1fx", target)
	default:
		return formatInt(int64(target))
	}
}

func formatCurrency(val float64) string {
	return fmt.Sprintf("Rs %s", formatInt(int64(math.Round(val))))
}

func costCell(spend float64, denom int64) SummaryCell {
	if spend <= 0 {
		return SummaryCell{Value: 0, Display: formatCurrency(0)}
	}
	if denom <= 0 {
		return SummaryCell{Value: 0, Display: "N/A"}
	}
	cost := spend / float64(denom)
	return SummaryCell{Value: cost, Display: formatCurrency(cost)}
}

func roasCell(revenue, spend float64) SummaryCell {
	if spend <= 0 || revenue <= 0 {
		return SummaryCell{Value: 0, Display: "N/A"}
	}
	roas := revenue / spend
	return SummaryCell{Value: roas, Display: fmt.Sprintf("%.1fx", roas)}
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

func toFloat(val any) float64 {
	switch v := val.(type) {
	case nil:
		return 0
	case float32:
		return float64(v)
	case float64:
		return v
	case int, int8, int16, int32, int64:
		return float64(toInt64(v))
	case uint, uint8, uint16, uint32, uint64:
		return float64(toInt64(v))
	case string:
		if s, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return s
		}
	case []byte:
		if s, err := strconv.ParseFloat(strings.TrimSpace(string(v)), 64); err == nil {
			return s
		}
	default:
		if s, err := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(v)), 64); err == nil {
			return s
		}
	}
	return 0
}

func makeCountCell(v int64) SummaryCell {
	return SummaryCell{
		Value:   float64(v),
		Display: formatInt(v),
	}
}

func makePercentCell(v float64) SummaryCell {
	return SummaryCell{
		Value:   v,
		Display: formatPercent(v),
	}
}

func currencyCell(amount float64) SummaryCell {
	if amount <= 0 {
		return SummaryCell{Value: 0, Display: formatCurrency(0)}
	}
	return SummaryCell{
		Value:   amount,
		Display: formatCurrency(amount),
	}
}

func naCell() SummaryCell {
	return SummaryCell{Value: 0, Display: "N/A"}
}

func dashCell() SummaryCell {
	return SummaryCell{Display: "—"}
}

func durationCell(avg float64, ok bool) SummaryCell {
	if !ok {
		return dashCell()
	}
	rounded := math.Round(avg*10) / 10
	return SummaryCell{
		Value:   rounded,
		Display: fmt.Sprintf("%.1f days", rounded),
	}
}

func getDate(val any) (time.Time, bool) {
	s := normalizeString(val)
	if s == "" {
		return time.Time{}, false
	}
	t, err := parseDate(s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func getDateFromFields(row map[string]any, keys ...string) (time.Time, bool) {
	for _, key := range keys {
		if row == nil {
			continue
		}
		if t, ok := getDate(row[key]); ok {
			return t, true
		}
	}
	return time.Time{}, false
}

func getDateConditional(row map[string]any, keys []string, flag any) (time.Time, bool) {
	if !isTruthy(flag) {
		return time.Time{}, false
	}
	return getDateFromFields(row, keys...)
}

func stringifyArgs(args []any) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, len(args))
	for i, v := range args {
		out[i] = fmt.Sprint(v)
	}
	return out
}

// func groupSourceName(raw string) string {
// 	raw = strings.TrimSpace(raw)
// 	if _, ok := metaSourceNames[raw]; ok {
// 		return metaGroupName
// 	}
// 	if raw == "" {
// 		return "Unknown"
// 	}
// 	return raw
// }

// func sourceGroupsMetadata() []SourceGroup {
// 	members := make([]string, 0, len(metaSourceNames))
// 	for name := range metaSourceNames {
// 		members = append(members, name)
// 	}
// 	if len(members) == 0 {
// 		return nil
// 	}
// 	sort.Strings(members)
//
// 	ids := make([]string, len(metaSourceIDs))
// 	copy(ids, metaSourceIDs)
//
// 	group := SourceGroup{
// 		Name:    metaGroupName,
// 		Sources: members,
// 	}
// 	if len(ids) > 0 {
// 		group.SourceIDs = ids
// 	}
// 	return []SourceGroup{group}
// }

func normalizeString(val any) string {
	switch v := val.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case []byte:
		return strings.TrimSpace(string(v))
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
