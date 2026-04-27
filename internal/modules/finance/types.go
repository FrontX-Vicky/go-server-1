package finance

import "time"

type OrderBy struct {
	Column    string `json:"column"`
	Direction string `json:"direction"`
}

type FranchiseeReportRequest struct {
	ReportID  int64     `json:"report_id"`
	StartDate string    `json:"start_date"`
	EndDate   string    `json:"end_date"`
	GroupBy   string    `json:"group_by"`
	Limit     int       `json:"limit"`
	Offset    int       `json:"offset"`
	OrderBy   []OrderBy `json:"order_by"`
	QueryOnly bool      `json:"query_only"`
}

type Column struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Position int    `json:"position"`
}

type Totals struct {
	TotalColumn string  `json:"total_column"`
	PageTotal   float64 `json:"page_total"`
	OverallTotal float64 `json:"overall_total"`
}

type Pagination struct {
	Limit      int `json:"limit"`
	Offset     int `json:"offset"`
	PageCount  int `json:"page_count"`
	TotalCount int `json:"total_count"`
}

type FranchiseeReportResponse struct {
	ReportID    int64              `json:"report_id"`
	Title       string             `json:"title"`
	Subtitle    string             `json:"subtitle"`
	ModuleID    int64              `json:"module_id"`
	Icon        string             `json:"icon"`
	StartDate   string             `json:"start_date"`
	EndDate     string             `json:"end_date"`
	Columns     []Column           `json:"columns"`
	Rows        []map[string]any   `json:"rows"`
	Totals      Totals             `json:"totals"`
	Pagination  Pagination         `json:"pagination"`
	GeneratedAt time.Time          `json:"generated_at"`
}
