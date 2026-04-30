package finance

import "time"

// ── Franchise Invoice types ────────────────────────────────────────────────

type FranchiseOwner struct {
	ID        int64  `json:"id"`
	OwnerName string `json:"owner_name"`
}

type ParticularItem struct {
	Amount float64 `json:"amount"`
	HSN    string  `json:"hsn"`
	IGST   float64 `json:"igst"`
	CGST   float64 `json:"cgst"`
	SGST   float64 `json:"sgst"`
}

type FranchiseInvoice struct {
	ID          int64                      `json:"id"`
	Branch      string                     `json:"branch"`
	MonthYear   string                     `json:"month_year"`
	InvoiceDate string                     `json:"invoice_date"`
	Invoice     string                     `json:"invoice"`
	Proforma    string                     `json:"proforma"`
	TotalSale   float64                    `json:"total_sale"`
	Royality    float64                    `json:"royality"`
	CGST        float64                    `json:"cgst"`
	SGST        float64                    `json:"sgst"`
	IGST        float64                    `json:"igst"`
	GrantTotal  float64                    `json:"grant_total"`
	OtherItems  string                     `json:"other_items"`
	Particulars map[string]*ParticularItem `json:"particulars,omitempty"`
}

type RoyaltyShare struct {
	TotalSale float64 `json:"total_sale"`
	Royalty   float64 `json:"royalty"`
	Month     string  `json:"month"`
	Branch    string  `json:"branch"`
}

type TaxData struct {
	CGSTTax string `json:"cgstTax"`
	SGSTTax string `json:"sgstTax"`
	IGSTTax string `json:"igstTax"`
}

type FranchiseInvoiceInitResponse struct {
	Owner        *FranchiseOwner `json:"owner"`
	Invoice      *FranchiseInvoice `json:"invoice,omitempty"`
	RoyaltyShare *RoyaltyShare   `json:"royalty_share,omitempty"`
	TaxData      *TaxData        `json:"tax_data,omitempty"`
}

type CreateFranchiseInvoiceRequest struct {
	OwnerID    int64   `json:"owner_id"`
	Branch     string  `json:"branch"`
	Invoice    string  `json:"invoice"`
	TotalSale  float64 `json:"total_sale"`
	Royality   float64 `json:"royality"`
	CGST       float64 `json:"cgst"`
	SGST       float64 `json:"sgst"`
	IGST       float64 `json:"igst"`
	GrantTotal float64 `json:"grant_total"`
	OtherItems string  `json:"other_items"`
	Month      string  `json:"month"`
	StartDate  string  `json:"start_date"`
	EndDate    string  `json:"end_date"`
	InvoiceDate string `json:"invoice_date"`
}

type UpdateFranchiseInvoiceRequest struct {
	InvoiceID   int64   `json:"invoice_id"`
	OwnerID     int64   `json:"owner_id"`
	Invoice     string  `json:"invoice"`
	Proforma    string  `json:"proforma"`
	GrantTotal  float64 `json:"grant_total"`
	OtherItems  string  `json:"other_items"`
	InvoiceDate string  `json:"invoice_date"`
}

type SubInvoice struct {
	ID               int64   `json:"id"`
	ParentInvoiceID  int64   `json:"parent_invoice_id"`
	Invoice          string  `json:"invoice"`
	InvoiceDate      string  `json:"invoice_date"`
	Proforma         string  `json:"proforma"`
	TotalSale        float64 `json:"total_sale"`
	Royality         float64 `json:"royality"`
	CGST             float64 `json:"cgst"`
	SGST             float64 `json:"sgst"`
	IGST             float64 `json:"igst"`
	GrantTotal       float64 `json:"grant_total"`
	OtherItems       string  `json:"other_items"`
	SalesInvoiceID   int64   `json:"sales_invoice_id"`
	SalesInvoiceNo   string  `json:"sales_invoice_no"`
	SalesInvoiceStatus string `json:"sales_invoice_status"`
	CreatedAt        string  `json:"created_at"`
	ItemLabel        string  `json:"item_label"`
	ItemHSN          string  `json:"item_hsn"`
	ItemGSTAmount    string  `json:"item_gst_amount"`
}

type CreateSubInvoiceRequest struct {
	ParentInvoiceID int64   `json:"parent_invoice_id"`
	Invoice         string  `json:"invoice"`
	InvoiceDate     string  `json:"invoice_date"`
	Proforma        string  `json:"proforma"`
	TotalSale       float64 `json:"total_sale"`
	Royality        float64 `json:"royality"`
	CGST            float64 `json:"cgst"`
	SGST            float64 `json:"sgst"`
	IGST            float64 `json:"igst"`
	CalculatedIGST  float64 `json:"calculated_igst"`
	GrantTotal      float64 `json:"grant_total"`
	OtherItems      string  `json:"other_items"`
}

type AnnexureSection struct {
	Key    string         `json:"key"`
	Label  string         `json:"label"`
	Rows   []map[string]any `json:"rows"`
	Totals map[string]any `json:"totals"`
}

type MemberTransferAnnexure struct {
	Title    string            `json:"title"`
	Sections []AnnexureSection `json:"sections"`
}

// ── Invoice list (royalty payment breakdown) ───────────────────────────────

type InvoiceListRow struct {
	InvoiceNo string  `json:"invoice_no"`
	FullName  string  `json:"full_name"`
	PayDate   string  `json:"pay_date"`
	PayMode   string  `json:"pay_mode"`
	Branch    string  `json:"branch"`
	Venue     string  `json:"venue"`
	Amount    float64 `json:"amount"`
	InvoiceID int64   `json:"invoice_id"`
}

type InvoiceBranchGroup struct {
	Branch string           `json:"branch"`
	Rows   []InvoiceListRow `json:"rows"`
	Total  float64          `json:"total"`
}

type InvoiceListResponse struct {
	Groups     []InvoiceBranchGroup `json:"groups"`
	GrandTotal float64              `json:"grand_total"`
}

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
	ForceRefresh bool   `json:"force_refresh"`
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
