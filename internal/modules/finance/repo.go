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
	158: {}, // IM Online
	307: {}, // IM Offline Venue
	209: {}, // IM Combine
	184: {}, // IM Offline Branch
	367: {}, // IM Report
	186: {}, // OM Online
	309: {}, // OM Offline Venue
	211: {}, // OM Combine
	185: {}, // OM Offline Branch
	366: {}, // OM Report
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

	// For a clean API, compute overall totals and count on the same query without paging.
	unpagedReq := req
	unpagedReq.Limit = 0
	unpagedReq.Offset = 0
	allRows, _, err := r.reportsRepo.RunReportQuery(ctx, req.ReportID, r.buildReportOptions(unpagedReq))
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
	overallTotal := sumColumn(allRows, totalColumn)

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
			TotalCount: len(allRows),
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
		}
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
