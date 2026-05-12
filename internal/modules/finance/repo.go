package finance

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"server_1/internal/modules/reports"
)

var supportedReportIDs = map[int64]struct{}{
	158: {}, // v1 Online
	307: {}, // v1 Offline Venue
	209: {}, // v1 Combine
	184: {}, // v1 Offline Branch
	367: {}, // v1 Report
	186: {}, // v2 Online
	309: {}, // v2 Offline Venue
	211: {}, // v2 Combine
	185: {}, // v2 Offline Branch
	366: {}, // v2 Report
}

// v2 report IDs run against DB2
var v2ReportIDs = map[int64]struct{}{
	186: {},
	309: {},
	211: {},
	185: {},
	366: {},
}

type Repo struct {
	reportsRepo *reports.Repo
}

func NewRepo() *Repo {
	return &Repo{reportsRepo: reports.NewRepo()}
}

func IsSupportedReportID(id int64) bool {
	_, ok := supportedReportIDs[id]
	return ok
}

func (r *Repo) BuildQuery(ctx context.Context, req FranchiseeReportRequest) (string, error) {
	opts := r.buildReportOptions(req)
	return r.reportsRepo.BuildReportQuery(ctx, req.ReportID, opts)
}

func (r *Repo) GetFranchiseeReport(ctx context.Context, req FranchiseeReportRequest) (*FranchiseeReportResponse, string, error) {
	meta, err := r.reportsRepo.Get(ctx, req.ReportID)
	if err != nil {
		return nil, "", err
	}

	opts := r.buildReportOptions(req)
	rows, query, err := r.reportsRepo.RunReportQuery(ctx, req.ReportID, opts)
	if err != nil {
		return nil, query, err
	}

	columns := make([]Column, 0, len(meta.Columns))
	for _, col := range meta.Columns {
		if strings.TrimSpace(col.ColumnName) == "" || col.Position == 0 {
			continue
		}
		columns = append(columns, Column{
			Key:      col.ColumnName,
			Label:    col.Header,
			Position: col.Position,
		})
	}

	totalColumn := strings.TrimSpace(meta.Total)
	pageTotal := sumColumn(rows, totalColumn)
	overallTotal := pageTotal
	totalCount := len(rows)

	if c, t, _, aggErr := r.reportsRepo.RunReportAggregates(ctx, req.ReportID, opts, totalColumn); aggErr == nil {
		totalCount = c
		overallTotal = t
	}

	resp := &FranchiseeReportResponse{
		ReportID:  req.ReportID,
		Title:     meta.Title,
		Subtitle:  meta.Subtitle,
		ModuleID:  meta.ModuleID,
		Icon:      meta.Icon,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
		Columns:   columns,
		Rows:      rows,
		Totals: Totals{
			TotalColumn:  totalColumn,
			PageTotal:    pageTotal,
			OverallTotal: overallTotal,
		},
		Pagination: Pagination{
			Limit:      req.Limit,
			Offset:     req.Offset,
			PageCount:  len(rows),
			TotalCount: totalCount,
		},
		GeneratedAt: time.Now().UTC(),
	}

	return resp, query, nil
}

func (r *Repo) buildReportOptions(req FranchiseeReportRequest) map[string]any {
	opts := map[string]any{
		"start_date": req.StartDate,
		"end_date":   req.EndDate,
		"groupby":    req.GroupBy,
	}

	if _, isV2 := v2ReportIDs[req.ReportID]; isV2 {
		opts["db_name"] = "DB2"
	}

	if req.Limit > 0 {
		opts["limit"] = strconv.Itoa(req.Limit)
	}
	if req.Offset > 0 {
		opts["offset"] = strconv.Itoa(req.Offset)
	}
	if len(req.OrderBy) > 0 {
		parts := make([]map[string]string, 0, len(req.OrderBy))
		for _, order := range req.OrderBy {
			col := strings.TrimSpace(order.Column)
			dir := strings.ToUpper(strings.TrimSpace(order.Direction))
			if col == "" {
				continue
			}
			if dir != "ASC" && dir != "DESC" {
				dir = "ASC"
			}
			parts = append(parts, map[string]string{col: dir})
		}
		if len(parts) > 0 {
			opts["orderby"] = parts
		} else {
			// All columns were blank; disable fallback to avoid invalid report_order entries
			opts["disable_order"] = true
		}
	} else {
		// No explicit order; disable the report_order fallback which may reference
		// columns not accessible at the ORDER BY level (e.g. subquery aliases).
		opts["disable_order"] = true
	}

	return opts
}

func sumColumn(rows []map[string]any, col string) float64 {
	if col == "" {
		return 0
	}
	total := 0.0
	for _, row := range rows {
		v, ok := row[col]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case int:
			total += float64(t)
		case int32:
			total += float64(t)
		case int64:
			total += float64(t)
		case float32:
			total += float64(t)
		case float64:
			total += t
		case []byte:
			if n, err := strconv.ParseFloat(string(t), 64); err == nil {
				total += n
			}
		case string:
			if n, err := strconv.ParseFloat(strings.TrimSpace(t), 64); err == nil {
				total += n
			}
		default:
			s := strings.TrimSpace(fmt.Sprint(t))
			if n, err := strconv.ParseFloat(s, 64); err == nil {
				total += n
			}
		}
	}
	return total
}
