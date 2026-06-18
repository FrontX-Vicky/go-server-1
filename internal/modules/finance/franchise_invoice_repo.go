package finance

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	coredb "server_1/internal/core/db"
)

type FranchiseInvoiceRepo struct {
	db1 *coredb.SQL
	db2 *coredb.SQL
}

func NewFranchiseInvoiceRepo() *FranchiseInvoiceRepo {
	return &FranchiseInvoiceRepo{
		db1: coredb.DBx("DB1"),
		db2: coredb.DBx("DB2"),
	}
}

// GetOwner fetches the owner name from branch_owner_master (DB1).
func (r *FranchiseInvoiceRepo) GetOwner(ctx context.Context, ownerID int64) (*FranchiseOwner, error) {
	row := r.db1.QueryRowContext(ctx,
		`SELECT id, COALESCE(owner_name,''), COALESCE(owner_contact_email,''), COALESCE(cc_email,''), COALESCE(bcc_email,'')
		 FROM branch_owner_master WHERE id = ? AND park = 0 LIMIT 1`,
		ownerID,
	)
	var o FranchiseOwner
	if err := row.Scan(&o.ID, &o.OwnerName, &o.ContactEmail, &o.CCEmail, &o.BCCEmail); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("owner %d not found", ownerID)
		}
		return nil, err
	}
	return &o, nil
}

// GetFranchiseInvoice fetches an existing invoice for the owner + month (DB1).
// monthYear must be formatted as "Jan 06" (e.g. "Mar 25").
func (r *FranchiseInvoiceRepo) GetFranchiseInvoice(ctx context.Context, ownerID int64, monthYear string) (*FranchiseInvoice, error) {
	row := r.db1.QueryRowContext(ctx,
		`SELECT id, COALESCE(branch,''), COALESCE(month_year,''), 
		        COALESCE(invoice_date,'0000-00-00'), COALESCE(invoice,''), COALESCE(proforma,'0'),
		        COALESCE(total_sale,0), COALESCE(royality,0),
		        COALESCE(cgst,0), COALESCE(sgst,0), COALESCE(igst,0),
		        COALESCE(grant_total,0), COALESCE(other_items,'')
		 FROM franchise_invoice
		 WHERE owner_name_id = ? AND park = 0 AND month_year = ?
		 ORDER BY id DESC LIMIT 1`,
		ownerID, monthYear,
	)
	var inv FranchiseInvoice
	if err := row.Scan(
		&inv.ID, &inv.Branch, &inv.MonthYear,
		&inv.InvoiceDate, &inv.Invoice, &inv.Proforma,
		&inv.TotalSale, &inv.Royality,
		&inv.CGST, &inv.SGST, &inv.IGST,
		&inv.GrantTotal, &inv.OtherItems,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No invoice yet
		}
		return nil, err
	}
	inv.Particulars = parseStoredParticulars(inv.OtherItems)
	return &inv, nil
}

// GetRoyaltyShare mirrors legacy getRoyaltyShare: read SQL from report_view_table
// by reportID, replace #sdate/#edate, run query, and pick the row for ownerID.
func (r *FranchiseInvoiceRepo) GetRoyaltyShare(ctx context.Context, ownerID, reportID int64, startDate, endDate string) (*RoyaltyShare, error) {
	if reportID <= 0 {
		return nil, fmt.Errorf("report_id is required for unsaved invoice init")
	}

	var viewSQL, subViewSQL string
	vRow := r.db1.QueryRowContext(ctx,
		`SELECT COALESCE(view,''), COALESCE(sub_view,'') FROM report_view_table WHERE r_id = ? LIMIT 1`,
		reportID,
	)
	if err := vRow.Scan(&viewSQL, &subViewSQL); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("report_view_table r_id=%d not found", reportID)
		}
		return nil, fmt.Errorf("report_view_table r_id=%d: %w", reportID, err)
	}

	if (reportID == 209 || reportID == 211) && strings.TrimSpace(subViewSQL) != "" {
		viewSQL = subViewSQL
	}
	if strings.TrimSpace(viewSQL) == "" {
		return nil, fmt.Errorf("report_view_table r_id=%d has empty SQL", reportID)
	}

	viewSQL = strings.ReplaceAll(viewSQL, "#sdate", startDate)
	viewSQL = strings.ReplaceAll(viewSQL, "#edate", endDate)

	rows, err := r.db1.QueryContext(ctx, viewSQL)
	if err != nil {
		return nil, fmt.Errorf("execute royalty view r_id=%d: %w", reportID, err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	raw := make([]sql.RawBytes, len(cols))
	dest := make([]any, len(cols))
	for i := range raw {
		dest[i] = &raw[i]
	}

	for rows.Next() {
		if err := rows.Scan(dest...); err != nil {
			continue
		}
		row := map[string]string{}
		for i, c := range cols {
			row[strings.ToLower(c)] = string(raw[i])
		}

		ownerVal := row["owner_id"]
		if ownerVal == "" {
			ownerVal = row["owner_name_id"]
		}
		if ownerVal == "" {
			ownerVal = row["ownerid"]
		}
		ownerRowID, _ := strconv.ParseInt(strings.TrimSpace(ownerVal), 10, 64)
		if ownerRowID != ownerID {
			continue
		}

		rs := &RoyaltyShare{
			TotalSale: toFloat(row["total_sale"]),
			Royalty:   toFloat(row["royalty"]),
			Month:     row["month"],
			Branch:    row["branch"],
		}
		if rs.Month == "" {
			rs.Month = row["month_year"]
		}
		if rs.Royalty == 0 {
			rs.Royalty = toFloat(row["royality"])
		}
		return rs, nil
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// No royalty-share row for this owner/report is a valid draft-init case.
	// Return zero values so UI can proceed and user can add manual particulars/sub-invoices.
	return &RoyaltyShare{}, nil
}

// GetTaxData fetches cgstTax, sgstTax, igstTax from branch table (DB1).
func (r *FranchiseInvoiceRepo) GetTaxData(ctx context.Context, branch string) (*TaxData, error) {
	row := r.db1.QueryRowContext(ctx,
		`SELECT COALESCE(cgstTax,'0'), COALESCE(sgstTax,'0'), COALESCE(igstTax,'0')
		 FROM branch WHERE branch = ? LIMIT 1`,
		branch,
	)
	var td TaxData
	if err := row.Scan(&td.CGSTTax, &td.SGSTTax, &td.IGSTTax); err != nil {
		if err == sql.ErrNoRows {
			return &TaxData{}, nil
		}
		return nil, err
	}
	return &td, nil
}

// CreateFranchiseInvoice inserts a new franchise_invoice row (DB1).
func (r *FranchiseInvoiceRepo) CreateFranchiseInvoice(ctx context.Context, req CreateFranchiseInvoiceRequest) (int64, error) {
	monthYear := strings.TrimSpace(req.Month)
	if monthYear == "" {
		monthYear = formatMonthYear(req.StartDate)
	}

	res, err := r.db1.ExecContext(ctx,
		`INSERT INTO franchise_invoice
		 (branch, owner_name_id, invoice, total_sale, royality, cgst, sgst, igst,
		  grant_total, other_items, month_year, start_date, end_date, invoice_date, park, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,NOW())`,
		req.Branch, req.OwnerID, req.Invoice, req.TotalSale,
		req.Royality, req.CGST, req.SGST, req.IGST,
		req.GrantTotal, req.OtherItems, monthYear,
		req.StartDate, req.EndDate, req.InvoiceDate,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateFranchiseInvoice updates an existing invoice (DB1).
func (r *FranchiseInvoiceRepo) UpdateFranchiseInvoice(ctx context.Context, req UpdateFranchiseInvoiceRequest) error {
	_, err := r.db1.ExecContext(ctx,
		`UPDATE franchise_invoice
		 SET invoice = ?, proforma = ?, grant_total = ?, other_items = ?, invoice_date = ?
		 WHERE id = ? AND owner_name_id = ? AND park = 0`,
		req.Invoice, req.Proforma, req.GrantTotal, req.OtherItems, req.InvoiceDate,
		req.InvoiceID, req.OwnerID,
	)
	return err
}

// DeleteFranchiseInvoice soft-deletes an invoice (park=1) in DB1.
func (r *FranchiseInvoiceRepo) DeleteFranchiseInvoice(ctx context.Context, invoiceID, ownerID int64) error {
	_, err := r.db1.ExecContext(ctx,
		`UPDATE franchise_invoice SET park = 1 WHERE id = ? AND owner_name_id = ? AND park = 0`,
		invoiceID, ownerID,
	)
	return err
}

// CreateSubInvoice inserts a sub-invoice row linked to the parent invoice (DB1).
func (r *FranchiseInvoiceRepo) CreateSubInvoice(ctx context.Context, req CreateSubInvoiceRequest) (int64, error) {
	// Load parent invoice fields required by legacy franchise_invoice_sub schema.
	parentRow := r.db1.QueryRowContext(ctx,
		`SELECT COALESCE(branch,''), COALESCE(owner_name_id,0), COALESCE(month_year,''),
		        COALESCE(start_date,''), COALESCE(end_date,''),
		        COALESCE(total_sale,0), COALESCE(royality,0),
		        COALESCE(cgst,0), COALESCE(sgst,0), COALESCE(igst,0),
		        COALESCE(NULLIF(calculated_igst,''),0), COALESCE(grant_total,0),
		        COALESCE(other_items,''), COALESCE(proforma,0),
		        COALESCE(invoice_date,'0000-00-00')
		 FROM franchise_invoice
		 WHERE id = ? AND park = 0`,
		req.ParentInvoiceID,
	)

	var (
		branch         string
		ownerNameID    int64
		monthYear      string
		startDate      string
		endDate        string
		parentTotal    float64
		parentRoyality float64
		parentCGST     float64
		parentSGST     float64
		parentIGST     float64
		parentCalcIGST float64
		parentGrant    float64
		parentItems    string
		parentProforma int64
		parentInvDate  string
	)
	if err := parentRow.Scan(
		&branch, &ownerNameID, &monthYear,
		&startDate, &endDate,
		&parentTotal, &parentRoyality,
		&parentCGST, &parentSGST, &parentIGST,
		&parentCalcIGST, &parentGrant,
		&parentItems, &parentProforma,
		&parentInvDate,
	); err != nil {
		return 0, fmt.Errorf("parent invoice %d not found: %w", req.ParentInvoiceID, err)
	}

	invoiceDate := req.InvoiceDate
	if strings.TrimSpace(invoiceDate) == "" || invoiceDate == "0000-00-00" {
		invoiceDate = parentInvDate
	}

	otherItems := req.OtherItems
	if strings.TrimSpace(otherItems) == "" {
		otherItems = parentItems
	}

	selectedLabel, selectedItem := extractFirstParticular(req.OtherItems)
	if selectedLabel == "" || selectedItem == nil {
		selectedLabel, selectedItem = extractFirstParticular(otherItems)
	}
	isRoyaltyItem := isRoyaltyParticular(selectedLabel)
	transferSectionKey := transferSectionKeyFromParticular(selectedLabel)

	totalSale := req.TotalSale
	if totalSale == 0 && isRoyaltyItem {
		totalSale = parentTotal
	}
	royality := req.Royality
	if royality == 0 && isRoyaltyItem {
		royality = parentRoyality
	}
	cgst := req.CGST
	if cgst == 0 && isRoyaltyItem {
		cgst = parentCGST
	}
	sgst := req.SGST
	if sgst == 0 && isRoyaltyItem {
		sgst = parentSGST
	}
	igst := req.IGST
	if igst == 0 && isRoyaltyItem {
		igst = parentIGST
	}
	calcIGST := req.CalculatedIGST
	if calcIGST == 0 && isRoyaltyItem {
		calcIGST = parentCalcIGST
	}

	// For transfer items (TTOC/TFOC), align GST percentages with the parent Royalty item.
	// This mirrors legacy behavior where transfer sub-invoices use royalty GST rates.
	if transferSectionKey != "" && selectedItem != nil {
		// Transfer lines must never persist as negative values in sub-invoices.
		selectedItem.Amount = roundFloat(math.Abs(selectedItem.Amount), 2)
		if selectedLabel != "" {
			if b, err := json.Marshal(map[string]*ParticularItem{selectedLabel: selectedItem}); err == nil {
				otherItems = string(b)
			}
		}

		parentParticulars := parseStoredParticulars(parentItems)
		if royaltyItem := parentParticulars["Royalty"]; royaltyItem != nil {
			selectedItem.CGST = royaltyItem.CGST
			selectedItem.SGST = royaltyItem.SGST
			selectedItem.IGST = royaltyItem.IGST

			cgst = roundFloat(selectedItem.Amount*selectedItem.CGST/100, 2)
			sgst = roundFloat(selectedItem.Amount*selectedItem.SGST/100, 2)
			igst = roundFloat(selectedItem.Amount*selectedItem.IGST/100, 2)
			calcIGST = igst

			if req.GrantTotal == 0 {
				grantFromRates := roundFloat(selectedItem.Amount+cgst+sgst+igst, 2)
				req.GrantTotal = grantFromRates
			}

		}
	}

	grantTotal := req.GrantTotal
	if grantTotal == 0 {
		if isRoyaltyItem {
			grantTotal = parentGrant
		} else if selectedItem != nil {
			grantTotal = roundFloat(selectedItem.Amount+cgst+sgst+igst, 2)
		} else {
			grantTotal = parentGrant
		}
	}
	if transferSectionKey != "" {
		grantTotal = roundFloat(math.Abs(grantTotal), 2)
	}
	proforma := req.Proforma
	if strings.TrimSpace(proforma) == "" {
		proforma = fmt.Sprintf("%d", parentProforma)
	}

	itemName, itemHSN, itemCGSTRate, itemSGSTRate, itemIGSTRate, itemGSTAmount := extractSubInvoiceItemSnapshot(otherItems)
	if strings.TrimSpace(req.ItemLabel) != "" {
		itemName = strings.TrimSpace(req.ItemLabel)
	}
	// Persist annexure_json on sub-invoice so sales invoice creation can reuse it directly.
	var annexureJSON string
	if req.ParentInvoiceID > 0 {
		if transferSectionKey != "" {
			if transferAnnexure, transferErr := r.GetMemberTransferAnnexure(ctx, ownerNameID, startDate, endDate); transferErr == nil && transferAnnexure != nil {
				if section, ok := pickAnnexureSection(transferAnnexure, transferSectionKey); ok {
					payload := &MemberTransferAnnexure{
						Title:    transferAnnexure.Title,
						Sections: []AnnexureSection{section},
					}
					if wrapped, jsonErr := encodeAnnexureJSON("member_transfer", payload); jsonErr == nil {
						annexureJSON = wrapped
					}
				}
			}
		} else if isRoyaltyItem {
			if annexureData, annexureErr := r.GetInvoiceList(ctx, req.ParentInvoiceID); annexureErr == nil && annexureData != nil {
				if wrapped, jsonErr := encodeAnnexureJSON("invoice_list", annexureData); jsonErr == nil {
					annexureJSON = wrapped
				}
			}
		}
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	insertCols := []string{
		"parent_invoice_id", "invoice", "branch", "owner_name_id", "month_year", "start_date", "end_date",
		"invoice_date", "total_sale", "royality", "cgst", "sgst", "igst", "calculated_igst",
		"item_name", "item_hsn", "item_cgst_rate", "item_sgst_rate", "item_igst_rate", "item_gst_amount",
		"other_items", "grant_total", "proforma", "created_by", "created_at", "modified_by", "modified_at",
	}
	insertArgs := []any{
		req.ParentInvoiceID, req.Invoice, branch, ownerNameID, monthYear, startDate, endDate,
		invoiceDate, totalSale, royality, cgst, sgst, igst, calcIGST,
		itemName, itemHSN, itemCGSTRate, itemSGSTRate, itemIGSTRate, itemGSTAmount,
		otherItems, grantTotal, proforma, 0, now, 0, now,
	}

	if subCols, colsErr := r.loadSubInvoiceCols(ctx); colsErr == nil && annexureJSON != "" {
		if subCols["annexure_json"] {
			insertCols = append(insertCols, "annexure_json")
			insertArgs = append(insertArgs, annexureJSON)
		}
	}

	ph := strings.TrimSuffix(strings.Repeat("?,", len(insertCols)), ",")
	insertQuery := `INSERT INTO franchise_invoice_sub (` + strings.Join(insertCols, ",") + `) VALUES (` + ph + `)`
	log.Printf("[finance.CreateSubInvoice] query=%s", insertQuery)
	log.Printf("[finance.CreateSubInvoice] placeholder_count=%d arg_count=%d", strings.Count(insertQuery, "?"), len(insertArgs))
	for i, arg := range insertArgs {
		log.Printf("[finance.CreateSubInvoice] arg[%02d]=%#v", i+1, arg)
	}

	res, err := r.db1.ExecContext(ctx, insertQuery, insertArgs...)
	if err != nil {
		log.Printf("[finance.CreateSubInvoice] exec error: %v", err)
		return 0, err
	}
	return res.LastInsertId()
}

// ListSubInvoices returns sub-invoices for a parent invoice (DB1).
func (r *FranchiseInvoiceRepo) ListSubInvoices(ctx context.Context, parentInvoiceID int64) ([]SubInvoice, error) {
	emailStatusExpr := `''`
	if r.hasEmailJobsTable(ctx) {
		emailStatusExpr = `COALESCE((
			SELECT ej.status
			FROM email_jobs ej
			WHERE ej.reference_type = 'sales_invoice'
			  AND ej.reference_id = CAST(s.sales_invoice_id AS CHAR)
			ORDER BY ej.id DESC
			LIMIT 1
		),'')`
	}
	rows, err := r.db1.QueryContext(ctx,
		`SELECT s.id, s.parent_invoice_id,
		        COALESCE(s.invoice,''), COALESCE(s.invoice_date,'0000-00-00'),
		        COALESCE(s.proforma,'0'), COALESCE(s.total_sale,0),
		        COALESCE(s.royality,0), COALESCE(s.cgst,0), COALESCE(s.sgst,0),
		        COALESCE(s.igst,0), COALESCE(s.grant_total,0), COALESCE(s.other_items,''),
		        COALESCE(s.sales_invoice_id,0), COALESCE(s.sales_invoice_no,''),
		        COALESCE(s.sales_invoice_status,''), COALESCE(m.document,''), `+emailStatusExpr+`, COALESCE(s.created_at,''),
		        COALESCE(s.item_name,''), COALESCE(s.item_hsn,''), COALESCE(s.item_gst_amount,0)
		 FROM franchise_invoice_sub s
		 LEFT JOIN sales_invoice_master m ON m.id = s.sales_invoice_id
		 WHERE s.parent_invoice_id = ? AND s.park = 0
		 ORDER BY id DESC`,
		parentInvoiceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []SubInvoice{}
	for rows.Next() {
		var (
			s            SubInvoice
			itemName     string
			itemHSN      string
			itemGSTValue float64
		)
		if err := rows.Scan(
			&s.ID, &s.ParentInvoiceID,
			&s.Invoice, &s.InvoiceDate,
			&s.Proforma, &s.TotalSale,
			&s.Royality, &s.CGST, &s.SGST,
			&s.IGST, &s.GrantTotal, &s.OtherItems,
			&s.SalesInvoiceID, &s.SalesInvoiceNo,
			&s.SalesInvoiceStatus, &s.SalesInvoiceDocument, &s.SalesInvoiceEmailStatus, &s.CreatedAt,
			&itemName, &itemHSN, &itemGSTValue,
		); err != nil {
			return nil, err
		}

		s.ItemLabel = itemName
		if strings.TrimSpace(s.ItemLabel) == "" {
			meta := extractSubInvoiceItemMeta(s.OtherItems)
			s.ItemLabel = meta[0]
			s.ItemHSN = meta[1]
			s.ItemGSTAmount = meta[2]
		} else {
			s.ItemHSN = itemHSN
			if strings.TrimSpace(s.ItemHSN) == "" {
				s.ItemHSN = "-"
			}
			s.ItemGSTAmount = fmt.Sprintf("%.2f", itemGSTValue)
		}

		list = append(list, s)
	}

	return list, rows.Err()
}

// UpdateSubInvoiceProforma updates a sub-invoice proforma flag and mirrors it to the linked sales invoice.
func (r *FranchiseInvoiceRepo) UpdateSubInvoiceProforma(ctx context.Context, subInvoiceID int64, proforma string) (err error) {
	if subInvoiceID <= 0 {
		return fmt.Errorf("valid sub_invoice_id is required")
	}

	proformaValue := "0"
	documentTypeCode := "INV"
	if isTruthyFlag(proforma) {
		proformaValue = "1"
		documentTypeCode = "PROFORMA"
	}

	masterCols, _, colsErr := r.loadSalesInvoiceCols(ctx)
	if colsErr != nil {
		masterCols = map[string]bool{}
	}

	tx, err := r.db1.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx,
		`UPDATE franchise_invoice_sub SET proforma = ?, modified_at = NOW() WHERE id = ? AND park = 0`,
		proformaValue,
		subInvoiceID,
	); err != nil {
		return fmt.Errorf("update franchise_invoice_sub.proforma: %w", err)
	}

	var salesInvoiceID int64
	if err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(sales_invoice_id,0) FROM franchise_invoice_sub WHERE id = ? AND park = 0 LIMIT 1`,
		subInvoiceID,
	).Scan(&salesInvoiceID); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("sub-invoice %d not found", subInvoiceID)
		}
		return fmt.Errorf("load sub-invoice sales invoice link: %w", err)
	}

	if salesInvoiceID > 0 {
		sets := []string{}
		args := []any{}
		if masterCols["proforma"] {
			sets = append(sets, "proforma = ?")
			args = append(args, proformaValue)
		}
		if masterCols["document_type_code"] {
			sets = append(sets, "document_type_code = ?")
			args = append(args, documentTypeCode)
		}
		if masterCols["modified_at"] {
			sets = append(sets, "modified_at = NOW()")
		}
		if len(sets) > 0 {
			args = append(args, salesInvoiceID)
			if _, err = tx.ExecContext(ctx,
				`UPDATE sales_invoice_master SET `+strings.Join(sets, ", ")+` WHERE id = ?`,
				args...,
			); err != nil {
				return fmt.Errorf("update linked sales_invoice_master proforma flag: %w", err)
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

// DeleteSubInvoice soft-deletes a sub-invoice (park=1) in DB1.
func (r *FranchiseInvoiceRepo) DeleteSubInvoice(ctx context.Context, subInvoiceID int64) error {
	_, err := r.db1.ExecContext(ctx,
		`UPDATE franchise_invoice_sub SET park = 1 WHERE id = ? AND park = 0`,
		subInvoiceID,
	)
	return err
}

// ─── sales invoice creation helpers ─────────────────────────────────────────

// franchiseSeries holds resolved series config data.
type franchiseSeries struct {
	ID                  int64
	Prefix              string
	Suffix              string
	Padding             int
	CollectionOwnerType string
	CollectionOwnerID   int64
}

// ownerDefaults holds owner/customer info from branch_owner_master.
type ownerDefaults struct {
	BranchID             int64
	OwnerName            string
	OwnerDisplayName     string
	OwnerCompanyName     string
	OwnerGSTIN           string
	OwnerState           string
	OwnerStateCode       string
	OwnerCountry         string
	BillingAddress       string
	BillingCity          string
	BillingPincode       string
	ShippingAddress      string
	ShippingCity         string
	ShippingPincode      string
	ContactPerson        string
	ContactEmail         string
	ContactPhone         string
	Currency             string
	TaxMode              string
	SupplyTypeCode       string
	PlaceOfSupply        string
	SupplyStateCode      string
	PaymentTerms         string
	Notes                string
	Terms                string
	SeriesConfigID       int64
	InvoiceCollectionMode string
}

type branchProfile struct {
	ID                int64
	InvoiceNameID     int64
	OwnerID           int64
	BranchName        string
	BranchInvoiceName string
	GSTNo             string
	InvoiceAddress    string
	InvoicePinCode    string
	City              string
	State             string
	Country           string
}

type supplierProfile struct {
	GSTIN        string
	PAN          string
	LegalName    string
	AddressLine1 string
	AddressLine2 string
	City         string
	State        string
	StateCode    string
	Country      string
	Pincode      string
}

// salesLineItem represents a single invoice line item.
type salesLineItem struct {
	Description  string
	HSN          string
	Quantity     float64
	Unit         string
	UnitPrice    float64
	TaxTreatment string // "taxable" | "exempt"
	GSTRate      float64
	CGSTRate     float64
	SGSTRate     float64
	IGSTRate     float64
	LineTotal    float64
	// Computed:
	TaxableAmount float64
	CGSTAmount    float64
	SGSTAmount    float64
	IGSTAmount    float64
	TotalTax      float64
}

// invoiceTotals holds computed invoice aggregate amounts.
type invoiceTotals struct {
	Subtotal       float64
	TaxableTotal   float64
	ExemptTotal    float64
	CGSTTotal      float64
	SGSTTotal      float64
	IGSTTotal      float64
	TotalTaxAmount float64
	GrandTotal     float64
}

// resolveFranchiseSeries returns the series config to use for a franchise invoice.
// If testMode=true it prefers a series with is_test=1; otherwise prefers is_default=1.
// Owner-specific series is checked first (invoice_collection_mode='owner').
func (r *FranchiseInvoiceRepo) resolveFranchiseSeries(ctx context.Context, ownerSeriesID int64, collectionMode string, testMode bool) (*franchiseSeries, error) {
	paddingExpr := `COALESCE(padding, 5)`

	// Try owner-specific series first (unless test mode forces test series).
	if !testMode && ownerSeriesID > 0 && strings.EqualFold(strings.TrimSpace(collectionMode), "owner") {
		row := r.db1.QueryRowContext(ctx,
			`SELECT id, COALESCE(prefix,''), COALESCE(suffix,''), `+paddingExpr+`,
			        COALESCE(collection_owner_type,'hq'), COALESCE(collection_owner_id,0)
			 FROM sales_invoice_series_config
			 WHERE id = ? AND active_flag = 1 LIMIT 1`,
			ownerSeriesID,
		)
		var s franchiseSeries
		err := row.Scan(&s.ID, &s.Prefix, &s.Suffix, &s.Padding, &s.CollectionOwnerType, &s.CollectionOwnerID)
		if err == nil && s.ID > 0 {
			return &s, nil
		}
	}

	// Fall back to company-level test or default series.
	var whereClause string
	if testMode {
		whereClause = `is_test = 1 AND active_flag = 1`
	} else {
		whereClause = `is_default = 1 AND active_flag = 1`
	}
	row := r.db1.QueryRowContext(ctx,
		`SELECT id, COALESCE(prefix,''), COALESCE(suffix,''), `+paddingExpr+`,
		        COALESCE(collection_owner_type,'hq'), COALESCE(collection_owner_id,0)
		 FROM sales_invoice_series_config
		 WHERE `+whereClause+`
		 ORDER BY id ASC LIMIT 1`,
	)
	var s franchiseSeries
	if err := row.Scan(&s.ID, &s.Prefix, &s.Suffix, &s.Padding, &s.CollectionOwnerType, &s.CollectionOwnerID); err != nil {
		if err == sql.ErrNoRows {
			if testMode {
				return nil, fmt.Errorf("no active test series found in sales_invoice_series_config (set is_test=1 on a series)")
			}
			return nil, fmt.Errorf("no active default series found in sales_invoice_series_config (set is_default=1 on a series)")
		}
		return nil, err
	}
	return &s, nil
}

// reserveInvoiceNumber atomically claims the next sequence number from the series
// and returns the formatted invoice number and the raw sequence number.
func (r *FranchiseInvoiceRepo) reserveInvoiceNumber(ctx context.Context, tx *sql.Tx, seriesID int64, prefix, suffix string, padding int) (invoiceNo string, sequenceNo int64, err error) {
	// Increment first, then read what we just claimed.
	if _, err = tx.ExecContext(ctx,
		`UPDATE sales_invoice_series_config SET next_number = next_number + 1 WHERE id = ?`,
		seriesID,
	); err != nil {
		return "", 0, fmt.Errorf("increment series %d: %w", seriesID, err)
	}
	row := tx.QueryRowContext(ctx,
		`SELECT next_number - 1 FROM sales_invoice_series_config WHERE id = ? FOR UPDATE`,
		seriesID,
	)
	if err = row.Scan(&sequenceNo); err != nil {
		return "", 0, fmt.Errorf("read sequence for series %d: %w", seriesID, err)
	}
	if padding < 1 {
		padding = 5
	}
	invoiceNo = fmt.Sprintf("%s%0*d%s", prefix, padding, sequenceNo, suffix)
	return invoiceNo, sequenceNo, nil
}

// fetchOwnerDefaults loads customer/owner info from branch_owner_master.
func (r *FranchiseInvoiceRepo) fetchOwnerDefaults(ctx context.Context, ownerID int64) (*ownerDefaults, error) {
	row := r.db1.QueryRowContext(ctx,
		`SELECT
		   COALESCE(NULLIF(TRIM(COALESCE(bid,'')),''), '0'),
		   COALESCE(owner_name,''),
		   COALESCE(owner_display_name,''),
		   COALESCE(owner_company_name,''),
		   COALESCE(owner_gstin,''),
		   COALESCE(owner_state,''),
		   COALESCE(owner_state_code,''),
		   COALESCE(owner_country,'India'),
		   COALESCE(owner_billing_address,''),
		   COALESCE(owner_billing_city,''),
		   COALESCE(owner_billing_pincode,''),
		   COALESCE(owner_shipping_address,''),
		   COALESCE(owner_shipping_city,''),
		   COALESCE(owner_shipping_pincode,''),
		   COALESCE(shipping_same_as_billing,0),
		   COALESCE(owner_contact_person,''),
		   COALESCE(owner_contact_email,''),
		   COALESCE(owner_contact_phone,''),
		   COALESCE(owner_currency,'INR'),
		   COALESCE(tax_mode,''),
		   COALESCE(supply_type_code,'B2B'),
		   COALESCE(place_of_supply,''),
		   COALESCE(supply_state_code,''),
		   COALESCE(payment_terms,'Due on Receipt'),
		   COALESCE(notes,''),
		   COALESCE(terms,''),
		   COALESCE(sales_invoice_series_config_id,0),
		   COALESCE(invoice_collection_mode,'owner')
		 FROM branch_owner_master
		 WHERE id = ? AND park = 0 LIMIT 1`,
		ownerID,
	)
	var d ownerDefaults
	var branchIDRaw string
	var shippingSameAsBilling int
	if err := row.Scan(
		&branchIDRaw,
		&d.OwnerName, &d.OwnerDisplayName, &d.OwnerCompanyName,
		&d.OwnerGSTIN, &d.OwnerState, &d.OwnerStateCode, &d.OwnerCountry,
		&d.BillingAddress, &d.BillingCity, &d.BillingPincode,
		&d.ShippingAddress, &d.ShippingCity, &d.ShippingPincode,
		&shippingSameAsBilling,
		&d.ContactPerson, &d.ContactEmail, &d.ContactPhone,
		&d.Currency, &d.TaxMode, &d.SupplyTypeCode,
		&d.PlaceOfSupply, &d.SupplyStateCode,
		&d.PaymentTerms, &d.Notes, &d.Terms,
		&d.SeriesConfigID, &d.InvoiceCollectionMode,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("owner %d not found in branch_owner_master", ownerID)
		}
		return nil, err
	}
	if parsedBranchID, parseErr := strconv.ParseInt(strings.TrimSpace(branchIDRaw), 10, 64); parseErr == nil {
		d.BranchID = parsedBranchID
	} else {
		d.BranchID = 0
	}
	if shippingSameAsBilling == 1 {
		d.ShippingAddress = d.BillingAddress
		d.ShippingCity = d.BillingCity
		d.ShippingPincode = d.BillingPincode
	}
	// Resolve display names.
	if d.OwnerDisplayName == "" {
		d.OwnerDisplayName = d.OwnerName
	}
	if d.OwnerCompanyName == "" {
		d.OwnerCompanyName = d.OwnerDisplayName
	}
	// Resolve place of supply defaults.
	if d.PlaceOfSupply == "" {
		if d.OwnerState != "" {
			d.PlaceOfSupply = d.OwnerState
		} else {
			d.PlaceOfSupply = "Maharashtra"
		}
	}
	if d.SupplyStateCode == "" {
		if d.OwnerStateCode != "" {
			d.SupplyStateCode = d.OwnerStateCode
		} else {
			d.SupplyStateCode = "27"
		}
	}
	return &d, nil
}

// parseFranchiseLineItems decodes other_items JSON and returns sales line items.
// Falls back to a single exempt line item using grantTotal if JSON cannot be parsed.
func parseFranchiseLineItems(otherItems string, grantTotal float64, preferredLabel string) []salesLineItem {
	var decoded map[string]map[string]any
	if otherItems != "" {
		if err := json.Unmarshal([]byte(otherItems), &decoded); err != nil {
			decoded = nil
		}
	}

	var items []salesLineItem
	if len(decoded) > 0 {
		usePreferredLabel := strings.TrimSpace(preferredLabel) != "" && len(decoded) == 1
		for label, item := range decoded {
			description := label
			if usePreferredLabel {
				description = strings.TrimSpace(preferredLabel)
			}
			amount := toFloat(item["amount"])
			if transferSectionKeyFromParticular(label) != "" || transferSectionKeyFromParticular(description) != "" {
				amount = roundFloat(math.Abs(amount), 2)
			}
			if amount == 0 {
				continue
			}
			igstRate := toFloat(item["igst"])
			cgstRate := toFloat(item["cgst"])
			sgstRate := toFloat(item["sgst"])
			gstRate := igstRate + cgstRate + sgstRate
			taxTreatment := "exempt"
			if gstRate > 0 {
				taxTreatment = "taxable"
			}
			hsn := strings.TrimSpace(fmt.Sprintf("%v", item["hsn"]))
			if hsn == "" || hsn == "<nil>" {
				hsn = defaultFranchiseHSN(label)
			}
			unit := "OTH"
			if strings.ToUpper(hsn) == "620342" {
				unit = "PCS"
			}
			items = append(items, salesLineItem{
				Description:  description,
				HSN:          hsn,
				Quantity:     1,
				Unit:         unit,
				UnitPrice:    amount,
				TaxTreatment: taxTreatment,
				GSTRate:      gstRate,
				CGSTRate:     cgstRate,
				SGSTRate:     sgstRate,
				IGSTRate:     igstRate,
				LineTotal:    roundFloat(amount, 2),
			})
		}
	}

	if len(items) == 0 {
		// Fallback single exempt line item.
		amt := roundFloat(grantTotal, 2)
		if transferSectionKeyFromParticular(preferredLabel) != "" {
			amt = roundFloat(math.Abs(amt), 2)
		}
		items = []salesLineItem{{
			Description:  firstNonEmpty(strings.TrimSpace(preferredLabel), "Franchisee invoice item"),
			HSN:          "9997",
			Quantity:     1,
			Unit:         "OTH",
			UnitPrice:    amt,
			TaxTreatment: "exempt",
			GSTRate:      0,
			LineTotal:    amt,
		}}
	}
	return items
}

// defaultFranchiseHSN returns the HSN/SAC for a known franchise line item label.
func defaultFranchiseHSN(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "shirt":
		return "620342"
	case "royalty", "royality":
		return "9997"
	default:
		return "9997"
	}
}

func normalizeComparableStateName(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return ""
	}
	return strings.Join(strings.Fields(v), " ")
}

func resolveFranchiseSalesTaxMode(requestedTaxMode, sellerState, sellerStateCode, buyerState, buyerStateCode string) string {
	taxMode := strings.ToLower(strings.TrimSpace(requestedTaxMode))
	if taxMode == "intra" || taxMode == "inter" {
		return taxMode
	}

	if sellerStateCode != "" && buyerStateCode != "" {
		if sellerStateCode == buyerStateCode {
			return "intra"
		}
		return "inter"
	}

	normalizedSellerState := normalizeComparableStateName(sellerState)
	normalizedBuyerState := normalizeComparableStateName(buyerState)
	if normalizedSellerState != "" && normalizedBuyerState != "" {
		if normalizedSellerState == normalizedBuyerState {
			return "intra"
		}
		return "inter"
	}

	return "intra"
}

// computeInvoiceTotals computes aggregate totals from line items exactly as
// they arrive from the sub-invoice payload.
func computeInvoiceTotals(items []salesLineItem) (invoiceTotals, []salesLineItem) {
	var t invoiceTotals
	computed := make([]salesLineItem, len(items))
	for i, item := range items {
		computed[i] = item
		t.Subtotal = roundFloat(t.Subtotal+item.UnitPrice*item.Quantity, 2)

		if item.TaxTreatment == "taxable" {
			taxable := roundFloat(item.UnitPrice*item.Quantity, 2)
			computed[i].TaxableAmount = taxable
			t.TaxableTotal = roundFloat(t.TaxableTotal+taxable, 2)
			computed[i].GSTRate = roundFloat(computed[i].CGSTRate+computed[i].SGSTRate+computed[i].IGSTRate, 2)
			computed[i].CGSTAmount = roundFloat(taxable*computed[i].CGSTRate/100, 2)
			computed[i].SGSTAmount = roundFloat(taxable*computed[i].SGSTRate/100, 2)
			computed[i].IGSTAmount = roundFloat(taxable*computed[i].IGSTRate/100, 2)
			t.CGSTTotal = roundFloat(t.CGSTTotal+computed[i].CGSTAmount, 2)
			t.SGSTTotal = roundFloat(t.SGSTTotal+computed[i].SGSTAmount, 2)
			t.IGSTTotal = roundFloat(t.IGSTTotal+computed[i].IGSTAmount, 2)
			totalTax := computed[i].CGSTAmount + computed[i].SGSTAmount + computed[i].IGSTAmount
			computed[i].TotalTax = roundFloat(totalTax, 2)
			computed[i].LineTotal = roundFloat(taxable+computed[i].TotalTax, 2)
		} else {
			exempt := roundFloat(item.UnitPrice*item.Quantity, 2)
			computed[i].GSTRate = 0
			computed[i].TaxableAmount = 0
			computed[i].CGSTRate = 0
			computed[i].SGSTRate = 0
			computed[i].IGSTRate = 0
			computed[i].CGSTAmount = 0
			computed[i].SGSTAmount = 0
			computed[i].IGSTAmount = 0
			computed[i].TotalTax = 0
			computed[i].LineTotal = exempt
			t.ExemptTotal = roundFloat(t.ExemptTotal+exempt, 2)
		}
	}
	t.TotalTaxAmount = roundFloat(t.CGSTTotal+t.SGSTTotal+t.IGSTTotal, 2)
	t.GrandTotal = roundFloat(t.TaxableTotal+t.ExemptTotal+t.TotalTaxAmount, 2)
	return t, computed
}

// salesInvoiceColumns queries and caches the column set for sales_invoice_master.
var siMasterCols map[string]bool
var siItemCols map[string]bool
var subInvoiceCols map[string]bool
var branchCols map[string]bool
var ownerMasterCols map[string]bool
var emailJobsTableAvailable *bool

func (r *FranchiseInvoiceRepo) loadSubInvoiceCols(ctx context.Context) (map[string]bool, error) {
	if subInvoiceCols != nil {
		return subInvoiceCols, nil
	}
	rows, err := r.db1.QueryContext(ctx, `SHOW COLUMNS FROM franchise_invoice_sub`)
	if err != nil {
		return nil, fmt.Errorf("SHOW COLUMNS franchise_invoice_sub: %w", err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var field, typ, null, key, extra string
		var def sql.NullString
		if err := rows.Scan(&field, &typ, &null, &key, &def, &extra); err != nil {
			return nil, err
		}
		cols[field] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	subInvoiceCols = cols
	return subInvoiceCols, nil
}

func (r *FranchiseInvoiceRepo) loadSalesInvoiceCols(ctx context.Context) (master map[string]bool, items map[string]bool, err error) {
	if siMasterCols != nil && siItemCols != nil {
		return siMasterCols, siItemCols, nil
	}
	loadCols := func(table string) (map[string]bool, error) {
		rows, err := r.db1.QueryContext(ctx, `SHOW COLUMNS FROM `+table)
		if err != nil {
			return nil, fmt.Errorf("SHOW COLUMNS %s: %w", table, err)
		}
		defer rows.Close()
		cols := map[string]bool{}
		for rows.Next() {
			var field, typ, null, key, extra string
			var def sql.NullString
			if err := rows.Scan(&field, &typ, &null, &key, &def, &extra); err != nil {
				return nil, err
			}
			cols[field] = true
		}
		return cols, rows.Err()
	}
	siMasterCols, err = loadCols("sales_invoice_master")
	if err != nil {
		return nil, nil, err
	}
	// sales_invoice_item may not exist in all deployments; treat gracefully.
	siItemCols, err = loadCols("sales_invoice_item")
	if err != nil {
		siItemCols = map[string]bool{}
	}
	return siMasterCols, siItemCols, nil
}

// roundFloat rounds f to d decimal places.
func roundFloat(f float64, d int) float64 {
	p := 1.0
	for range make([]struct{}, d) {
		p *= 10
	}
	return float64(int(f*p+0.5)) / p
}

var gstinPattern = regexp.MustCompile(`^[0-9]{2}[A-Z]{5}[0-9]{4}[A-Z][1-9A-Z]Z[0-9A-Z]$`)
var panPattern = regexp.MustCompile(`^[A-Z]{5}[0-9]{4}[A-Z]$`)

func normalizeGSTIN(value string) string {
	v := strings.ToUpper(strings.TrimSpace(value))
	if gstinPattern.MatchString(v) {
		return v
	}
	return ""
}

func normalizePAN(value string) string {
	v := strings.ToUpper(strings.TrimSpace(value))
	if panPattern.MatchString(v) {
		return v
	}
	return ""
}

func stateCodeFromGSTIN(gstin string) string {
	gstin = normalizeGSTIN(gstin)
	if len(gstin) >= 2 {
		return gstin[:2]
	}
	return ""
}

func firstNonZero(values ...int64) int64 {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func (r *FranchiseInvoiceRepo) loadBranchCols(ctx context.Context) (map[string]bool, error) {
	if branchCols != nil {
		return branchCols, nil
	}
	rows, err := r.db1.QueryContext(ctx, `SHOW COLUMNS FROM branch`)
	if err != nil {
		return nil, fmt.Errorf("SHOW COLUMNS branch: %w", err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var field, typ, null, key, extra string
		var def sql.NullString
		if err := rows.Scan(&field, &typ, &null, &key, &def, &extra); err != nil {
			return nil, err
		}
		cols[field] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	branchCols = cols
	return branchCols, nil
}

func (r *FranchiseInvoiceRepo) loadOwnerMasterCols(ctx context.Context) (map[string]bool, error) {
	if ownerMasterCols != nil {
		return ownerMasterCols, nil
	}
	rows, err := r.db1.QueryContext(ctx, `SHOW COLUMNS FROM branch_owner_master`)
	if err != nil {
		return nil, fmt.Errorf("SHOW COLUMNS branch_owner_master: %w", err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var field, typ, null, key, extra string
		var def sql.NullString
		if err := rows.Scan(&field, &typ, &null, &key, &def, &extra); err != nil {
			return nil, err
		}
		cols[field] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	ownerMasterCols = cols
	return ownerMasterCols, nil
}

func (r *FranchiseInvoiceRepo) hasEmailJobsTable(ctx context.Context) bool {
	if emailJobsTableAvailable != nil {
		return *emailJobsTableAvailable
	}
	row := r.db1.QueryRowContext(ctx, `SHOW TABLES LIKE 'email_jobs'`)
	var tableName string
	ok := false
	if err := row.Scan(&tableName); err == nil && strings.TrimSpace(tableName) != "" {
		ok = true
	}
	emailJobsTableAvailable = &ok
	return ok
}

func (r *FranchiseInvoiceRepo) fetchBranchProfile(ctx context.Context, branchID int64) (*branchProfile, error) {
	cols, err := r.loadBranchCols(ctx)
	if err != nil {
		return nil, err
	}
	exprOrDefault := func(col string, fallback string) string {
		if cols[col] {
			return "COALESCE(" + col + "," + fallback + ")"
		}
		return fallback
	}
	selects := []string{
		"id",
		exprOrDefault("branch", "''"),
		exprOrDefault("branch_invoice_name", "''"),
		exprOrDefault("gst_no", "''"),
		exprOrDefault("invoice_address", "''"),
		exprOrDefault("invoice_pin_code", "''"),
		exprOrDefault("city", "''"),
		exprOrDefault("state", "''"),
		exprOrDefault("country", "'India'"),
	}
	if cols["invoice_name_id"] {
		selects = append(selects, "COALESCE(invoice_name_id,0)")
	} else {
		selects = append(selects, "0")
	}
	if cols["owner_id"] {
		selects = append(selects, "COALESCE(owner_id,0)")
	} else {
		selects = append(selects, "0")
	}

	row := r.db1.QueryRowContext(ctx,
		`SELECT `+strings.Join(selects, ",")+` FROM branch WHERE id = ? LIMIT 1`,
		branchID,
	)
	var b branchProfile
	if err := row.Scan(
		&b.ID,
		&b.BranchName,
		&b.BranchInvoiceName,
		&b.GSTNo,
		&b.InvoiceAddress,
		&b.InvoicePinCode,
		&b.City,
		&b.State,
		&b.Country,
		&b.InvoiceNameID,
		&b.OwnerID,
	); err != nil {
		return nil, fmt.Errorf("branch %d not found: %w", branchID, err)
	}
	return &b, nil
}

func (r *FranchiseInvoiceRepo) fetchSupplierProfile(ctx context.Context, ownerID int64) (*supplierProfile, error) {
	if ownerID <= 0 {
		return &supplierProfile{Country: "India"}, nil
	}
	cols, err := r.loadOwnerMasterCols(ctx)
	if err != nil {
		return nil, err
	}
	panExpr := `''`
	for _, candidate := range []string{"supplier_pan", "owner_pancard", "pan", "pan_no", "pan_number", "pan_card"} {
		if cols[candidate] {
			panExpr = "COALESCE(" + candidate + ",'')"
			break
		}
	}
	selects := []string{
		"COALESCE(owner_name,'')",
		"COALESCE(owner_display_name,'')",
		"COALESCE(owner_company_name,'')",
		"COALESCE(owner_gstin,'')",
		"COALESCE(owner_state,'')",
		"COALESCE(owner_country,'India')",
		"COALESCE(owner_billing_address,'')",
		"COALESCE(owner_billing_city,'')",
		"COALESCE(owner_billing_pincode,'')",
		panExpr,
	}
	row := r.db1.QueryRowContext(ctx,
		`SELECT `+strings.Join(selects, ",")+` FROM branch_owner_master WHERE id = ? AND park = 0 LIMIT 1`,
		ownerID,
	)
	var ownerName, ownerDisplayName, ownerCompanyName, ownerGSTIN, ownerState, ownerCountry, billingAddress, billingCity, billingPincode, ownerPAN string
	if err := row.Scan(
		&ownerName,
		&ownerDisplayName,
		&ownerCompanyName,
		&ownerGSTIN,
		&ownerState,
		&ownerCountry,
		&billingAddress,
		&billingCity,
		&billingPincode,
		&ownerPAN,
	); err != nil {
		if err == sql.ErrNoRows {
			return &supplierProfile{Country: "India"}, nil
		}
		return nil, fmt.Errorf("fetch supplier owner profile %d: %w", ownerID, err)
	}
	gstin := normalizeGSTIN(ownerGSTIN)
	return &supplierProfile{
		GSTIN:        gstin,
		PAN:          normalizePAN(ownerPAN),
		LegalName:    firstNonEmpty(ownerCompanyName, ownerDisplayName, ownerName),
		AddressLine1: strings.TrimSpace(billingAddress),
		AddressLine2: "",
		City:         strings.TrimSpace(billingCity),
		State:        strings.TrimSpace(ownerState),
		StateCode:    stateCodeFromGSTIN(gstin),
		Country:      firstNonEmpty(ownerCountry, "India"),
		Pincode:      strings.TrimSpace(billingPincode),
	}, nil
}

func encodeAnnexureJSON(annexureType string, payload any) (string, error) {
	envelope := map[string]any{
		"type":    strings.TrimSpace(annexureType),
		"version": 1,
		"data":    payload,
	}
	b, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func extractFirstParticular(raw string) (string, *ParticularItem) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	var parsed map[string]map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return "", nil
	}
	for label, v := range parsed {
		item := &ParticularItem{
			Amount: toFloat(v["amount"]),
			HSN:    toString(v["hsn"]),
			IGST:   toFloat(v["igst"]),
			CGST:   toFloat(v["cgst"]),
			SGST:   toFloat(v["sgst"]),
		}
		return strings.TrimSpace(label), item
	}
	return "", nil
}

func isRoyaltyParticular(label string) bool {
	v := strings.ToLower(strings.TrimSpace(label))
	return v == "royalty" || v == "royality"
}

func transferSectionKeyFromParticular(label string) string {
	v := strings.ToUpper(strings.TrimSpace(label))
	if strings.HasPrefix(v, "TTOC") {
		return "TTOC"
	}
	if strings.HasPrefix(v, "TFOC") {
		return "TFOC"
	}
	return ""
}

func pickAnnexureSection(a *MemberTransferAnnexure, key string) (AnnexureSection, bool) {
	if a == nil {
		return AnnexureSection{}, false
	}
	target := strings.ToUpper(strings.TrimSpace(key))
	for _, s := range a.Sections {
		if strings.ToUpper(strings.TrimSpace(s.Key)) == target {
			return s, true
		}
	}
	return AnnexureSection{}, false
}

// CreateSalesInvoiceFromSub creates a full sales invoice from a sub-invoice.
// Mirrors PHP's frenchisee_sub_invoice_create_sales_invoice → salesInvoice::save_invoice().
// testMode=true selects the test invoice series (is_test=1) instead of the default series.
// A real DB record is always created regardless of testMode.
func (r *FranchiseInvoiceRepo) CreateSalesInvoiceFromSub(ctx context.Context, subInvoiceID int64, testMode bool) (int64, error) {
	// 1. Fetch sub-invoice.
	subRow := r.db1.QueryRowContext(ctx,
		`SELECT id, COALESCE(parent_invoice_id,0), COALESCE(owner_name_id,0), COALESCE(invoice_date,'0000-00-00'),
		        COALESCE(grant_total,0), COALESCE(other_items,''), COALESCE(item_name,''), COALESCE(proforma,'0')
		 FROM franchise_invoice_sub WHERE id = ? AND park = 0`,
		subInvoiceID,
	)
	var (
		subID           int64
		parentInvoiceID int64
		ownerID         int64
		invoiceDate     string
		grantTotal      float64
		otherItems      string
		itemName        string
		proforma        string
	)
	if err := subRow.Scan(&subID, &parentInvoiceID, &ownerID, &invoiceDate, &grantTotal, &otherItems, &itemName, &proforma); err != nil {
		return 0, fmt.Errorf("sub-invoice %d not found: %w", subInvoiceID, err)
	}
	if invoiceDate == "" || invoiceDate == "0000-00-00" {
		invoiceDate = time.Now().Format("2006-01-02")
	}

	// Prefer annexure JSON already stored on sub-invoice.
	storedAnnexureJSON := ""
	if subCols, colsErr := r.loadSubInvoiceCols(ctx); colsErr == nil {
		if subCols["annexure_json"] {
			annexureRow := r.db1.QueryRowContext(ctx,
				`SELECT COALESCE(annexure_json, '') FROM franchise_invoice_sub WHERE id = ? AND park = 0 LIMIT 1`,
				subInvoiceID,
			)
			_ = annexureRow.Scan(&storedAnnexureJSON)
		}
	}

	// 2. Fetch owner defaults (customer snapshot).
	owner, err := r.fetchOwnerDefaults(ctx, ownerID)
	if err != nil {
		return 0, err
	}

	// 3. Load column sets (cached).
	masterCols, itemCols, err := r.loadSalesInvoiceCols(ctx)
	if err != nil {
		return 0, err
	}

	// 4. Resolve series.
	// Franchise sub-invoice sales invoices are always created in the B2B flow,
	// so they should follow the company sales-invoice settings just like the
	// legacy PHP resolver does. Owner-mapped series should not override the
	// company default/test series in this path.
	series, err := r.resolveFranchiseSeries(ctx, 0, "company", testMode)
	if err != nil {
		return 0, err
	}

		// 5. Parse line items.
		lineItems := parseFranchiseLineItems(otherItems, grantTotal, itemName)

		// 5b. Fetch annexure (invoice list) from the parent franchise_invoice.
		// Store as JSON in annexure_json so the invoice print can render Annexure 1.
		annexureJSON := strings.TrimSpace(storedAnnexureJSON)
	if annexureJSON == "" && parentInvoiceID > 0 {
		annexureData, annexureErr := r.GetInvoiceList(ctx, parentInvoiceID)
		if annexureErr == nil && annexureData != nil {
			if wrapped, jsonErr := encodeAnnexureJSON("invoice_list", annexureData); jsonErr == nil {
				annexureJSON = wrapped
			}
		}
		// Non-fatal: if annexure fetch fails the invoice is still created.
	}

		// 6. Resolve supplier branch profile and tax mode before computing invoice totals.
		// Franchise sub-invoice sales invoices are booked to bid 35 in test mode, else bid 27.
		branchID := int64(27)
	if testMode {
		branchID = 35
	}
	branchProfile, err := r.fetchBranchProfile(ctx, branchID)
	if err != nil {
		return 0, err
	}
	supplierOwnerID := firstNonZero(branchProfile.InvoiceNameID, branchProfile.OwnerID)
	supplier, err := r.fetchSupplierProfile(ctx, supplierOwnerID)
	if err != nil {
		return 0, err
	}
	if supplier.GSTIN == "" {
		supplier.GSTIN = normalizeGSTIN(branchProfile.GSTNo)
	}
	if supplier.LegalName == "" {
		supplier.LegalName = firstNonEmpty(branchProfile.BranchInvoiceName, branchProfile.BranchName)
	}
	if supplier.AddressLine1 == "" {
		supplier.AddressLine1 = strings.TrimSpace(branchProfile.InvoiceAddress)
	}
	if supplier.City == "" {
		supplier.City = strings.TrimSpace(branchProfile.City)
	}
	if supplier.State == "" {
		supplier.State = strings.TrimSpace(branchProfile.State)
	}
	if supplier.Country == "" {
		supplier.Country = firstNonEmpty(branchProfile.Country, "India")
	}
	if supplier.Pincode == "" {
		supplier.Pincode = strings.TrimSpace(branchProfile.InvoicePinCode)
	}
		if supplier.StateCode == "" {
			supplier.StateCode = stateCodeFromGSTIN(supplier.GSTIN)
		}

		totals, lineItems := computeInvoiceTotals(lineItems)

		// 7. Begin transaction.
		tx, err := r.db1.BeginTx(ctx, nil)
		if err != nil {
			return 0, err
		}
		defer func() {
			if err != nil {
				_ = tx.Rollback()
			}
		}()

		// 8. Reserve invoice number.
		invoiceNo, sequenceNo, err := r.reserveInvoiceNumber(ctx, tx, series.ID, series.Prefix, series.Suffix, series.Padding)
		if err != nil {
			return 0, err
		}

		// 9. Build sales_invoice_master field map.
		now := time.Now().Format("2006-01-02 15:04:05")
	fields := map[string]any{
		"branch_id":              branchID,
		"invoice_no":             invoiceNo,
		"display_invoice_no":     invoiceNo,
		"auto_number":            1,
		"series_config_id":       series.ID,
		"sequence_no":            sequenceNo,
		"prefix_snapshot":        series.Prefix,
		"suffix_snapshot":        series.Suffix,
		"owner_type":             series.CollectionOwnerType,
		"owner_id":               nullableInt(series.CollectionOwnerID),
		"owner_id_resolved":      nullableInt(ownerID),
		"owner_resolution_source": "owner",
		"series_resolution_source": func() string {
			if testMode {
				return "b2b_test_series"
			}
			return "b2b_company_default"
		}(),
		"billing_context_type":   "b2b",
		"source_type":            "franchisee_sub_invoice",
		"source_id":              subID,
		"document_number":        invoiceNo,
		"document_type_code": func() string {
			if isTruthyFlag(proforma) {
				return "PROFORMA"
			}
			return "INV"
		}(),
		"document_date":          invoiceDate,
		"customer_display_name":  owner.OwnerDisplayName,
		"customer_company_name":  owner.OwnerCompanyName,
		"customer_gstin":         owner.OwnerGSTIN,
		"customer_state":         owner.OwnerState,
		"customer_state_code":    owner.OwnerStateCode,
		"customer_country":       owner.OwnerCountry,
		"customer_billing_address":  owner.BillingAddress,
		"customer_billing_city":     owner.BillingCity,
		"customer_billing_pincode":  owner.BillingPincode,
		"customer_shipping_address": owner.ShippingAddress,
		"customer_shipping_city":    owner.ShippingCity,
		"customer_shipping_pincode": owner.ShippingPincode,
		"customer_contact_person": owner.ContactPerson,
		"customer_contact_email":  owner.ContactEmail,
		"customer_contact_phone":  owner.ContactPhone,
		"generated_by_gstin":      supplier.GSTIN,
		"supplier_gstin":          supplier.GSTIN,
		"supplier_pan":            supplier.PAN,
		"supplier_legal_name":     supplier.LegalName,
		"supplier_address_line_1": supplier.AddressLine1,
		"supplier_address_line_2": supplier.AddressLine2,
		"supplier_city":           supplier.City,
		"supplier_state":          supplier.State,
		"supplier_state_code":     supplier.StateCode,
		"supplier_country":        supplier.Country,
		"supplier_pincode":        supplier.Pincode,
		"recipient_gstin":         owner.OwnerGSTIN,
		"recipient_name":          firstNonEmpty(owner.OwnerDisplayName, owner.OwnerCompanyName, owner.OwnerName),
		"recipient_billing_address_line_1": owner.BillingAddress,
		"recipient_billing_address_line_2": "",
		"recipient_city":          owner.BillingCity,
		"recipient_state":         owner.OwnerState,
		"recipient_state_code":    firstNonEmpty(owner.OwnerStateCode, stateCodeFromGSTIN(owner.OwnerGSTIN)),
		"recipient_country":       firstNonEmpty(owner.OwnerCountry, "India"),
		"recipient_pincode":       owner.BillingPincode,
		"shipping_name":           firstNonEmpty(owner.OwnerDisplayName, owner.OwnerCompanyName, owner.OwnerName),
		"shipping_address_line_1": owner.ShippingAddress,
		"shipping_address_line_2": "",
		"shipping_city":           owner.ShippingCity,
		"shipping_state":          owner.OwnerState,
		"shipping_state_code":     firstNonEmpty(owner.OwnerStateCode, stateCodeFromGSTIN(owner.OwnerGSTIN)),
		"shipping_country":        firstNonEmpty(owner.OwnerCountry, "India"),
		"shipping_pincode":        owner.ShippingPincode,
		"customer_currency":       owner.Currency,
		"place_of_supply":         owner.PlaceOfSupply,
		"supply_state_code":       owner.SupplyStateCode,
			"supply_type_code":        owner.SupplyTypeCode,
		"invoice_date":            invoiceDate,
		"due_date":                invoiceDate,
		"payment_terms":           owner.PaymentTerms,
		"notes":                   owner.Notes,
		"terms":                   owner.Terms,
		"status":                  "draft",
		"tax_profile_type": func() string {
			if totals.TaxableTotal > 0 || totals.TotalTaxAmount > 0 {
				return "taxable"
			}
			return "exempt"
		}(),
		"is_tax_exempt": func() int {
			if totals.TaxableTotal > 0 || totals.TotalTaxAmount > 0 {
				return 0
			}
			return 1
		}(),
		"currency_code":           owner.Currency,
		"currency_rate":           1.0,
		"subtotal_amount":         totals.Subtotal,
		"taxable_amount":          totals.TaxableTotal,
		"exempt_amount":           totals.ExemptTotal,
		"cgst_amount":             totals.CGSTTotal,
		"sgst_amount":             totals.SGSTTotal,
		"igst_amount":             totals.IGSTTotal,
		"total_tax_amount":        totals.TotalTaxAmount,
		"grand_total":             totals.GrandTotal,
		"total_amount":            totals.GrandTotal,
		"total_invoice_amount":    totals.GrandTotal,
		"paid_amount":             0.0,
		"balance_amount":          totals.GrandTotal,
		"balance_due":             totals.GrandTotal,
		"payment_status":          "unpaid",
		"park":                    0,
			"created_at":              now,
		}
	// Store annexure rows if we fetched them successfully.
	if annexureJSON != "" && masterCols["annexure_json"] {
		fields["annexure_json"] = annexureJSON
	}
	if masterCols["proforma"] {
		if isTruthyFlag(proforma) {
			fields["proforma"] = "1"
		} else {
			fields["proforma"] = "0"
		}
	}

	// 9. Filter to only existing columns.
	cols := []string{}
	vals := []any{}
	for col, val := range fields {
		if masterCols[col] {
			cols = append(cols, "`"+col+"`")
			vals = append(vals, val)
		}
	}
	placeholders := strings.Repeat("?,", len(cols))
	placeholders = strings.TrimSuffix(placeholders, ",")
	insertSQL := `INSERT INTO sales_invoice_master (` + strings.Join(cols, ",") + `) VALUES (` + placeholders + `)`

	res, err := tx.ExecContext(ctx, insertSQL, vals...)
	if err != nil {
		return 0, fmt.Errorf("insert sales_invoice_master: %w", err)
	}
	salesInvoiceID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	// 10. Insert line items (if table/columns exist).
	if len(itemCols) > 0 {
		for index, item := range lineItems {
			itemFields := map[string]any{
				"invoice_id":       salesInvoiceID,
				"line_order":       index + 1,
				"description":      item.Description,
				"hsn_sac":          item.HSN,
				"quantity":         item.Quantity,
				"unit":             item.Unit,
				"unit_price":       item.UnitPrice,
				"discount_type":    "amount",
				"discount_value": 0.0,
				"discount_amount":  0.0,
				"tax_treatment":    item.TaxTreatment,
				"gst_rate":         item.GSTRate,
				"tax_rate":         item.GSTRate,
				"taxable_value":    item.TaxableAmount,
				"taxable_amount":   item.TaxableAmount,
				"cgst_rate":        item.CGSTRate,
				"sgst_rate":        item.SGSTRate,
				"igst_rate":        item.IGSTRate,
				"cgst_amount":      item.CGSTAmount,
				"sgst_amount":      item.SGSTAmount,
				"igst_amount":      item.IGSTAmount,
				"tax_amount":       item.TotalTax,
				"total_tax":        item.TotalTax,
				"other_charges":    0.0,
				"line_total":       item.LineTotal,
				"total_amount":     item.LineTotal,
				"park":             0,
				"created_at":       now,
			}
			iCols := []string{}
			iVals := []any{}
			for col, val := range itemFields {
				if itemCols[col] {
					iCols = append(iCols, "`"+col+"`")
					iVals = append(iVals, val)
				}
			}
			if len(iCols) > 0 {
				ph := strings.TrimSuffix(strings.Repeat("?,", len(iCols)), ",")
				_, err = tx.ExecContext(ctx, `INSERT INTO sales_invoice_item (`+strings.Join(iCols, ",")+`) VALUES (`+ph+`)`, iVals...)
				if err != nil {
					return 0, fmt.Errorf("insert sales_invoice_item: %w", err)
				}
			}
		}
	}

	// 11. Update franchise_invoice_sub with the new sales invoice reference.
	if _, err = tx.ExecContext(ctx,
		`UPDATE franchise_invoice_sub
		 SET sales_invoice_id = ?, sales_invoice_no = ?, sales_invoice_status = 'created',
		     sales_invoice_created_at = ?, modified_at = ?
		 WHERE id = ? AND park = 0`,
		salesInvoiceID, invoiceNo, now, now, subID,
	); err != nil {
		return 0, fmt.Errorf("update franchise_invoice_sub: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}
	if strings.EqualFold(strings.TrimSpace(owner.SupplyTypeCode), "B2B") && masterCols["document"] {
		if _, persistErr := r.persistLegacyB2BSalesInvoiceDocument(ctx, salesInvoiceID, invoiceNo); persistErr != nil {
			log.Printf("persist legacy b2b sales invoice document failed for sales_invoice_id=%d: %v", salesInvoiceID, persistErr)
		}
	}
	return salesInvoiceID, nil
}

const b2bSalesInvoiceDocumentDir = "/var/www/content/pdf/TREPL"

const legacyFinanceBaseURL = "https://admin.tickleright.in"

func sanitizeDocumentSegment(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "invoice"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_")
	return replacer.Replace(trimmed)
}

func (r *FranchiseInvoiceRepo) persistLegacyB2BSalesInvoiceDocument(ctx context.Context, salesInvoiceID int64, invoiceNo string) (string, error) {
	return r.persistLegacyB2BSalesInvoiceDocumentWithMode(ctx, salesInvoiceID, invoiceNo, false)
}

func (r *FranchiseInvoiceRepo) persistLegacyB2BSalesInvoiceDocumentWithMode(ctx context.Context, salesInvoiceID int64, invoiceNo string, revisioned bool) (string, error) {
	if salesInvoiceID <= 0 || strings.TrimSpace(invoiceNo) == "" {
		return "", fmt.Errorf("sales invoice id and invoice no are required")
	}
	if err := os.MkdirAll(b2bSalesInvoiceDocumentDir, 0o755); err != nil {
		return "", fmt.Errorf("create b2b invoice document dir: %w", err)
	}
	pdfName := fmt.Sprintf("SalesInvoice_%s", invoiceNo)
	query := url.Values{}
	query.Set("test", "pdf_test")
	query.Set("view", "1")
	query.Set("email_body_filename", "salesInvoice")
	if proforma, _ := r.isSalesInvoiceProforma(ctx, salesInvoiceID); proforma {
		query.Set("pdf_body_filename", "salesInvoiceProforma")
	} else {
		query.Set("pdf_body_filename", "salesInvoice")
	}
	query.Set("folder_name", "sales_invoices")
	query.Set("email_table_name", "sales_invoice_master")
	query.Set("pdf_name", pdfName)
	query.Set("mail", "0")
	query.Set("table_row_id", strconv.FormatInt(salesInvoiceID, 10))

	legacyURL := legacyFinanceBaseURL + "/classes/tester.php?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, legacyURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "MarkX-B2BInvoiceDocument/1.0")
	req.Header.Set("Accept", "application/pdf,*/*")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch legacy sales invoice pdf: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("legacy sales invoice pdf returned %s", resp.Status)
	}
	pdfBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read legacy sales invoice pdf: %w", err)
	}
	if len(pdfBytes) < 4 || string(pdfBytes[:4]) != "%PDF" {
		return "", fmt.Errorf("legacy response is not a PDF")
	}

	fileName := fmt.Sprintf("SalesInvoice_%s.pdf", sanitizeDocumentSegment(invoiceNo))
	if revisioned {
		fileName = fmt.Sprintf(
			"SalesInvoice_%s_rev_%s.pdf",
			sanitizeDocumentSegment(invoiceNo),
			time.Now().Format("20060102_150405"),
		)
	}
	fullPath := filepath.Join(b2bSalesInvoiceDocumentDir, fileName)
	if err := os.WriteFile(fullPath, pdfBytes, 0o644); err != nil {
		return "", fmt.Errorf("write legacy sales invoice pdf: %w", err)
	}

	if _, err := r.db1.ExecContext(ctx,
		`UPDATE sales_invoice_master SET document = ? WHERE id = ?`,
		fullPath, salesInvoiceID,
	); err != nil {
		_ = os.Remove(fullPath)
		return "", fmt.Errorf("update sales_invoice_master.document: %w", err)
	}
	return fullPath, nil
}

func isTruthyFlag(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized == "1" || normalized == "true" || normalized == "yes" || normalized == "y"
}

func (r *FranchiseInvoiceRepo) isSalesInvoiceProforma(ctx context.Context, salesInvoiceID int64) (bool, error) {
	if salesInvoiceID <= 0 {
		return false, nil
	}
	masterCols, _, err := r.loadSalesInvoiceCols(ctx)
	if err != nil {
		return false, err
	}
	if masterCols["proforma"] {
		var proforma string
		row := r.db1.QueryRowContext(ctx, `SELECT COALESCE(proforma,'0') FROM sales_invoice_master WHERE id = ? LIMIT 1`, salesInvoiceID)
		if err := row.Scan(&proforma); err != nil {
			return false, err
		}
		if isTruthyFlag(proforma) {
			return true, nil
		}
	}
	if masterCols["document_type_code"] {
		var documentType string
		row := r.db1.QueryRowContext(ctx, `SELECT COALESCE(document_type_code,'') FROM sales_invoice_master WHERE id = ? LIMIT 1`, salesInvoiceID)
		if err := row.Scan(&documentType); err != nil {
			return false, err
		}
		return strings.EqualFold(strings.TrimSpace(documentType), "PROFORMA"), nil
	}
	return false, nil
}

func (r *FranchiseInvoiceRepo) GetSalesInvoiceNumber(ctx context.Context, salesInvoiceID int64) (string, error) {
	if salesInvoiceID <= 0 {
		return "", fmt.Errorf("valid sales invoice id is required")
	}
	row := r.db1.QueryRowContext(
		ctx,
		`SELECT COALESCE(invoice_no,'') FROM sales_invoice_master WHERE id = ? LIMIT 1`,
		salesInvoiceID,
	)
	var invoiceNo string
	if err := row.Scan(&invoiceNo); err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("sales invoice %d not found", salesInvoiceID)
		}
		return "", fmt.Errorf("fetch sales invoice no: %w", err)
	}
	invoiceNo = strings.TrimSpace(invoiceNo)
	if invoiceNo == "" {
		return "", fmt.Errorf("sales invoice %d has empty invoice_no", salesInvoiceID)
	}
	return invoiceNo, nil
}

func (r *FranchiseInvoiceRepo) RegenerateSalesInvoiceDocument(ctx context.Context, salesInvoiceID int64, invoiceNo string) (string, string, error) {
	resolvedInvoiceNo := strings.TrimSpace(invoiceNo)
	if resolvedInvoiceNo == "" {
		var err error
		resolvedInvoiceNo, err = r.GetSalesInvoiceNumber(ctx, salesInvoiceID)
		if err != nil {
			return "", "", err
		}
	}
	attachmentPath, err := r.persistLegacyB2BSalesInvoiceDocumentWithMode(ctx, salesInvoiceID, resolvedInvoiceNo, true)
	if err != nil {
		return "", "", err
	}
	return attachmentPath, resolvedInvoiceNo, nil
}

// nullableInt returns nil (NULL) for zero values, otherwise returns the int64.
func nullableInt(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func extractSubInvoiceItemSnapshot(raw string) (itemName, itemHSN string, itemCGSTRate, itemSGSTRate, itemIGSTRate, itemGSTAmount float64) {
	meta := extractSubInvoiceItemMeta(raw)
	itemName = strings.TrimSpace(meta[0])
	if itemName == "-" {
		itemName = ""
	}
	itemHSN = strings.TrimSpace(meta[1])
	if itemHSN == "-" {
		itemHSN = ""
	}

	var parsed map[string]map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		if gst, err2 := strconv.ParseFloat(meta[2], 64); err2 == nil {
			itemGSTAmount = gst
		}
		return
	}

	for _, item := range parsed {
		amount := toFloat(item["amount"])
		itemCGSTRate = toFloat(item["cgst"])
		itemSGSTRate = toFloat(item["sgst"])
		itemIGSTRate = toFloat(item["igst"])
		itemGSTAmount = amount*itemCGSTRate/100 + amount*itemSGSTRate/100 + amount*itemIGSTRate/100
		break
	}

	if itemGSTAmount == 0 {
		if gst, err := strconv.ParseFloat(meta[2], 64); err == nil {
			itemGSTAmount = gst
		}
	}

	return
}

// GetInvoiceList returns the per-invoice royalty payment breakdown for a franchise invoice.
// Mirrors PHP invoiceFrenchisee.php: fetches report_view_table r_id=300002, replaces
// #sdate / #edate / #id, executes it to get invoice_ids, then queries invoice_payment_franchisee_view.
func (r *FranchiseInvoiceRepo) GetInvoiceList(ctx context.Context, franchiseInvoiceID int64) (*InvoiceListResponse, error) {
	empty := &InvoiceListResponse{Groups: []InvoiceBranchGroup{}, GrandTotal: 0}

	// 1. Fetch parent invoice to get owner + date range.
	var ownerID int64
	var startDate, endDate string
	row := r.db1.QueryRowContext(ctx,
		`SELECT COALESCE(owner_name_id,0), COALESCE(start_date,''), COALESCE(end_date,'')
		 FROM franchise_invoice WHERE id = ? AND park = 0 LIMIT 1`,
		franchiseInvoiceID,
	)
	if err := row.Scan(&ownerID, &startDate, &endDate); err != nil {
		return nil, fmt.Errorf("franchise invoice %d not found: %w", franchiseInvoiceID, err)
	}

	// 2. Get report view SQL from report_view_table r_id=300002.
	var viewSQL string
	vRow := r.db1.QueryRowContext(ctx,
		`SELECT COALESCE(view,'') FROM report_view_table WHERE r_id = 300002 LIMIT 1`)
	if err := vRow.Scan(&viewSQL); err != nil {
		return nil, fmt.Errorf("report_view_table r_id=300002: %w", err)
	}
	if strings.TrimSpace(viewSQL) == "" {
		return empty, nil
	}
	viewSQL = strings.ReplaceAll(viewSQL, "#sdate", startDate)
	viewSQL = strings.ReplaceAll(viewSQL, "#edate", endDate)
	viewSQL = strings.ReplaceAll(viewSQL, "#id", fmt.Sprintf("%d", ownerID))

	// 3. Execute view SQL to get comma-separated invoice_ids.
	// Wrap in a subquery so we always scan exactly one column (invoice_ids).
	// Use QueryContext (retry-enabled) so a stale pool connection triggers a fresh-conn retry.
	wrappedSQL := "SELECT invoice_ids FROM (" + viewSQL + ") AS _view_wrapper LIMIT 1"
	idRows, err := r.db1.QueryContext(ctx, wrappedSQL)
	if err != nil {
		return nil, fmt.Errorf("execute view SQL: %w", err)
	}
	var invoiceIDsNull sql.NullString
	if idRows.Next() {
		if scanErr := idRows.Scan(&invoiceIDsNull); scanErr != nil {
			idRows.Close()
			return nil, fmt.Errorf("scan invoice_ids: %w", scanErr)
		}
	}
	idRows.Close()
	if err := idRows.Err(); err != nil {
		return nil, fmt.Errorf("invoice_ids rows error: %w", err)
	}
	if !invoiceIDsNull.Valid || strings.TrimSpace(invoiceIDsNull.String) == "" {
		return empty, nil
	}
	invoiceIDsRaw := invoiceIDsNull.String

	// Validate: only digits and commas (guard against SQL injection from DB data).
	for _, ch := range invoiceIDsRaw {
		if ch != ',' && (ch < '0' || ch > '9') {
			return empty, nil
		}
	}

	// 4. Fetch invoice payment details.
	// Mirrors PHP: _.amount in invoice_payment_franchisee_view is the invoice amount column.
	// CASE condition has NO Coupon filter (matches PHP invoiceFrenchisee.php exactly).
	dataQ := `SELECT
		COALESCE(_.invoice,''),
		COALESCE(_.fullname,''),
		COALESCE(_.pay_date_string,''),
		COALESCE(_.pay_mode_text,''),
		COALESCE(_.branch,''),
		COALESCE(_.venue,''),
		_.invoice_id,
		(CASE
		    WHEN _.bid = 9 THEN COALESCE(_.calculated_amount, _.pay_amount)
		    WHEN (COALESCE(_.tax,0) > 0 OR COALESCE(_.item_disallowed_discount,0) > 0 OR COALESCE(_.item_discount,0) > 0)
		    THEN _.amount - COALESCE(_.item_discount,0) + COALESCE(_.item_disallowed_discount,0)
		    ELSE COALESCE(_.pay_amount, 0)
		END) - COALESCE(p.amount, 0) AS pay_amount
	FROM invoice_payment_franchisee_view AS _
	LEFT JOIN (
		SELECT invoice_id, SUM(amount) AS amount
		FROM payment WHERE pay_mode_text = 'Points'
		GROUP BY invoice_id
	) p ON p.invoice_id = _.invoice_id
	WHERE _.invoice_id IN (` + invoiceIDsRaw + `)
	  AND _.pay_mode_text != 'Points'
	ORDER BY _.branch, _.venue ASC`

	rows, err := r.db1.QueryContext(ctx, dataQ)
	if err != nil {
		return nil, fmt.Errorf("invoice_payment_franchisee_view query: %w", err)
	}
	defer rows.Close()

	// 5. Group by branch.
	groupMap := map[string]*InvoiceBranchGroup{}
	var groupOrder []string
	var grandTotal float64

	for rows.Next() {
		var row InvoiceListRow
		if err := rows.Scan(&row.InvoiceNo, &row.FullName, &row.PayDate, &row.PayMode,
			&row.Branch, &row.Venue, &row.InvoiceID, &row.Amount); err != nil {
			continue
		}
		row.Amount = roundFloat(row.Amount, 2)
		if _, ok := groupMap[row.Branch]; !ok {
			groupMap[row.Branch] = &InvoiceBranchGroup{Branch: row.Branch, Rows: []InvoiceListRow{}}
			groupOrder = append(groupOrder, row.Branch)
		}
		groupMap[row.Branch].Rows = append(groupMap[row.Branch].Rows, row)
		groupMap[row.Branch].Total = roundFloat(groupMap[row.Branch].Total+row.Amount, 2)
		grandTotal = roundFloat(grandTotal+row.Amount, 2)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("scan invoice list rows: %w", err)
	}

	groups := make([]InvoiceBranchGroup, 0, len(groupOrder))
	for _, branch := range groupOrder {
		groups = append(groups, *groupMap[branch])
	}
	return &InvoiceListResponse{Groups: groups, GrandTotal: grandTotal}, nil
}

// GetMemberTransferAnnexure returns transfer annexure data from DB1.
// Mirrors PHP invoiceFrenchisee.php check_out (TTOC) and check_in (TFOC).
func (r *FranchiseInvoiceRepo) GetMemberTransferAnnexure(ctx context.Context, ownerID int64, startDate, endDate string) (*MemberTransferAnnexure, error) {
	empty := &MemberTransferAnnexure{Title: "Member Transfer Annexure", Sections: []AnnexureSection{}}

	// ── Step 1: get owner's branches ────────────────────────────────────────
	brows, err := r.db1.QueryContext(ctx,
		`SELECT id FROM branch WHERE invoice_name_id = ? AND park = 0`, ownerID)
	if err != nil {
		return empty, nil
	}
	var branchIDs []int64
	for brows.Next() {
		var id int64
		if err := brows.Scan(&id); err == nil {
			branchIDs = append(branchIDs, id)
		}
	}
	_ = brows.Close()
	if len(branchIDs) == 0 {
		return empty, nil
	}

	inPh := strings.TrimSuffix(strings.Repeat("?,", len(branchIDs)), ",")
	branchArgs := make([]any, len(branchIDs))
	for i, id := range branchIDs {
		branchArgs[i] = id
	}

	// ── Step 2: fetch TTOC and TFOC transfers ───────────────────────────────
	// TTOC = check_out: kid transferred FROM owner's branch (from_bid IN owner branches)
	// TFOC = check_in:  kid transferred TO   owner's branch (to_bid   IN owner branches)
	type transferRow struct {
		InvoiceID     int64
		ContactID     int64
		MemberName    string
		FromBidName   string
		ToBidName     string
		FromOwnerID   int64
		ToOwnerID     int64
		FromCatMaster int
		ToCatMaster   int
		FromBatch     int64
		ToBatch       int64
		FromBid       int64
		ToBid         int64
		ActionDate    string
	}

	fetchTransfers := func(bidCol string) ([]transferRow, error) {
		q := `SELECT mt.invoice_id, mt.contact_id, mt.member_name,
		             COALESCE(mt.from_bid_name,''), COALESCE(mt.to_bid_name,''),
		             mt.from_owner_id, mt.to_owner_id,
		             COALESCE(mt.from_category_master,0), COALESCE(mt.to_category_master,0),
		             COALESCE(mt.from_batch,0), COALESCE(mt.to_batch,0),
		             COALESCE(mt.from_bid,0), COALESCE(mt.to_bid,0), mt.action_date
		      FROM member_transfer_view mt
		      WHERE mt.status = '1' AND mt.action_date BETWEEN ? AND ?
		        AND mt.` + bidCol + ` IN (` + inPh + `) AND mt.park = 0`
		args := append([]any{startDate, endDate}, branchArgs...)
		rows, err := r.db1.QueryContext(ctx, q, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var result []transferRow
		for rows.Next() {
			var t transferRow
			if err := rows.Scan(
				&t.InvoiceID, &t.ContactID, &t.MemberName,
				&t.FromBidName, &t.ToBidName,
				&t.FromOwnerID, &t.ToOwnerID,
				&t.FromCatMaster, &t.ToCatMaster,
				&t.FromBatch, &t.ToBatch,
				&t.FromBid, &t.ToBid, &t.ActionDate,
			); err != nil {
				continue
			}
			result = append(result, t)
		}
		return result, rows.Err()
	}

	ttocTransfers, err := fetchTransfers("from_bid")
	if err != nil {
		ttocTransfers = nil
	}
	tfocTransfers, err := fetchTransfers("to_bid")
	if err != nil {
		tfocTransfers = nil
	}

	allTransfers := append(ttocTransfers, tfocTransfers...)
	if len(allTransfers) == 0 {
		return empty, nil
	}

	// ── Step 3: collect unique invoice_ids ──────────────────────────────────
	invoiceSet := map[int64]bool{}
	for _, t := range allTransfers {
		invoiceSet[t.InvoiceID] = true
	}

	// ── Step 4: invoice payment data for all invoice IDs ────────────────────
	// Keep row-level results instead of collapsing by invoice_id so this mirrors
	// the legacy PHP loop structure in invoiceFrenchisee.php.
	type invoiceData struct {
		InvoiceNo       string
		StartDate       string
		ServiceSessions int64
		Amount          float64
	}
	invoiceRowsByInvoice := map[int64][]invoiceData{}
	if len(invoiceSet) > 0 {
		invIDs := make([]any, 0, len(invoiceSet))
		invPh := ""
		for id := range invoiceSet {
			invIDs = append(invIDs, id)
			invPh += "?,"
		}
		invPh = strings.TrimSuffix(invPh, ",")
		invQ := `SELECT ii.invoice_id, ii.invoice, ii.start_date,
		                COALESCE(si.sessions, 0),
		                (CASE
		                    WHEN p.bid = 9 THEN COALESCE(p.actual_amount, p.amount)
		                    WHEN (COALESCE(ii.tax_amount,0) > 0 OR COALESCE(ii.item_disallowed_discount,0) > 0 OR COALESCE(ii.item_discount,0) > 0)
		                         AND COALESCE(p.pay_mode_text,'') != 'Coupon'
		                    THEN ii.invoice_amount + COALESCE(ii.item_disallowed_discount,0) - COALESCE(ii.item_discount,0)
		                    ELSE COALESCE(p.amount, 0)
		                END) - COALESCE(pp.amount, 0)
		         FROM invoice_invoiceitem_view ii
		         LEFT JOIN service si ON si.id = ii.service_id
		         LEFT JOIN payment p ON p.invoice_id = ii.invoice_id
		         LEFT JOIN (SELECT invoice_id, SUM(amount) AS amount FROM payment WHERE pay_mode_text = 'Points' GROUP BY invoice_id) pp
		                ON pp.invoice_id = ii.invoice_id
		         WHERE ii.invoice_id IN (` + invPh + `)
		           AND ii.park = 0 AND ii.master_category_id IN (1, 2) AND ii.bid NOT IN (35)
		         ORDER BY ii.invoice_id DESC`
		invRows, err := r.db1.QueryContext(ctx, invQ, invIDs...)
		if err == nil {
			for invRows.Next() {
				var iid int64
				var d invoiceData
				if err := invRows.Scan(&iid, &d.InvoiceNo, &d.StartDate, &d.ServiceSessions, &d.Amount); err == nil {
					invoiceRowsByInvoice[iid] = append(invoiceRowsByInvoice[iid], d)
				}
			}
			invRows.Close()
		}
	}

	// ── Step 5: royalty rates by branch_id and batch_id ─────────────────────
	// TTOC: uses from_bid (online) and from_batch venue (offline), checks from_category_master
	// TFOC: uses to_bid  (online) and to_batch  venue (offline), checks to_category_master
	onlineRoyaltyMap := map[int64]float64{}
	offlineRoyaltyMap := map[int64]float64{}

	bidSet := map[int64]bool{}
	batchSet := map[int64]bool{}
	for _, t := range allTransfers {
		if t.FromBid > 0 {
			bidSet[t.FromBid] = true
		}
		if t.ToBid > 0 {
			bidSet[t.ToBid] = true
		}
		if t.FromBatch > 0 {
			batchSet[t.FromBatch] = true
		}
		if t.ToBatch > 0 {
			batchSet[t.ToBatch] = true
		}
	}
	if len(bidSet) > 0 {
		bidList := make([]any, 0, len(bidSet))
		bph := ""
		for id := range bidSet {
			bidList = append(bidList, id)
			bph += "?,"
		}
		bph = strings.TrimSuffix(bph, ",")
		royaltyRows, err := r.db1.QueryContext(ctx, `SELECT id, COALESCE(royalty,0) FROM branch WHERE id IN (`+bph+`)`, bidList...)
		if err == nil {
			for royaltyRows.Next() {
				var bid int64
				var pct float64
				if err := royaltyRows.Scan(&bid, &pct); err == nil {
					onlineRoyaltyMap[bid] = pct
				}
			}
			royaltyRows.Close()
		}
	}
	if len(batchSet) > 0 {
		batchList := make([]any, 0, len(batchSet))
		batchPh := ""
		for id := range batchSet {
			batchList = append(batchList, id)
			batchPh += "?,"
		}
		if len(batchList) > 0 {
			batchPh = strings.TrimSuffix(batchPh, ",")
			venueRows, err := r.db1.QueryContext(ctx,
				`SELECT bat.id, COALESCE(v.royalty, 0) FROM batch bat
				 JOIN venue v ON v.id = bat.venue_id
				 WHERE bat.id IN (`+batchPh+`)`, batchList...)
			if err == nil {
				defer venueRows.Close()
				for venueRows.Next() {
					var batchID int64
					var pct float64
					if err := venueRows.Scan(&batchID, &pct); err == nil {
						offlineRoyaltyMap[batchID] = pct
					}
				}
			}
		}
	}

	// ── Step 6: attendance query helper (mirrors PHP per invoice row) ───────
	type attRow struct {
		Date   string
		Bid    int64
		Branch string
	}
	loadAttendanceRows := func(contactID int64, invoiceStartDate, actionDate string) []attRow {
		invoiceStartDate = normalizeDateOnly(invoiceStartDate)
		actionDate = normalizeDateOnly(actionDate)
		rows, err := r.db1.QueryContext(ctx,
			`SELECT COALESCE(date,''), COALESCE(bid,0), COALESCE(branch,'')
			 FROM attendance_cont_view
			 WHERE contact_id = ? AND park = 0 AND date BETWEEN ? AND ?`,
			contactID, invoiceStartDate, actionDate,
		)
		if err != nil {
			return nil
		}
		defer rows.Close()

		result := []attRow{}
		for rows.Next() {
			var a attRow
			if err := rows.Scan(&a.Date, &a.Bid, &a.Branch); err == nil {
				result = append(result, a)
			}
		}
		return result
	}

	// ── Step 7: build TTOC rows (PHP check_out) ──────────────────────────────
	// kid transferred OUT from owner's branch → owner RECEIVES money for remaining sessions
	// online royalty from from_bid, offline from from_batch venue, check from_category_master
	// attendance: date < action_date  OR  (date == action_date AND bid != to_bid)
	buildTTOCRows := func(transfers []transferRow) ([]map[string]any, float64, float64) {
		var total float64
		var gstCreditBalance float64
		rows := []map[string]any{}
		for _, t := range transfers {
			invoiceRows := invoiceRowsByInvoice[t.InvoiceID]
			if len(invoiceRows) == 0 {
				continue
			}
			for _, invData := range invoiceRows {
				if invData.ServiceSessions <= 0 {
					continue
				}
				actionDate := normalizeDateOnly(t.ActionDate)
				royaltyPct := onlineRoyaltyMap[t.FromBid]
				if t.FromCatMaster == 2 {
					if pct, ok := offlineRoyaltyMap[t.FromBatch]; ok {
						royaltyPct = pct
					}
				}
				royaltyAmount := invData.Amount * (float64(100-royaltyPct) / 100)
				perSession := royaltyAmount / float64(invData.ServiceSessions)
				attendanceRows := loadAttendanceRows(t.ContactID, invData.StartDate, actionDate)

				var prevCount int64
				branchCount := map[string]int{}
				branchOrder := []string{}
				for _, a := range attendanceRows {
					attDate := normalizeDateOnly(a.Date)
					if attDate < actionDate {
						prevCount++
						if _, ok := branchCount[a.Branch]; !ok {
							branchOrder = append(branchOrder, a.Branch)
						}
						branchCount[a.Branch]++
					} else if attDate == actionDate && a.Bid != t.ToBid {
						prevCount++
						if _, ok := branchCount[a.Branch]; !ok {
							branchOrder = append(branchOrder, a.Branch)
						}
						branchCount[a.Branch]++
					}
				}
				// TTOC is receivable; do not allow negative pending sessions to
				// turn receivable into a negative amount.
				pendingSession := invData.ServiceSessions - prevCount
				if pendingSession < 0 {
					pendingSession = 0
				}
				forwardAmt := roundFloat(perSession*float64(pendingSession), 0)

				sessionsDone := []string{}
				for _, branch := range branchOrder {
					sessionsDone = append(sessionsDone, fmt.Sprintf("%s - %d", branch, branchCount[branch]))
				}

				sameOwner := 0
				if t.FromOwnerID == t.ToOwnerID {
					sameOwner = 1
				} else {
					total += forwardAmt
					gstCreditBalance += forwardAmt * 0.18
				}
				// PHP check_out: always appends (same_owner rows show 0 amounts in UI)
				rows = append(rows, map[string]any{
					"invoice_no":      invData.InvoiceNo,
					"name":            t.MemberName,
					"from_bid":        t.FromBidName,
					"to_bid":          t.ToBidName,
					"total_session":   invData.ServiceSessions,
					"amount":          roundFloat(invData.Amount, 2),
					"share":           roundFloat(royaltyAmount, 2),
					"pending_session": pendingSession,
					"forward":         forwardAmt,
					"same_owner":      sameOwner,
					"sessions_done":   sessionsDone,
				})
			}
		}
		return rows, total, roundFloat(gstCreditBalance, 2)
	}

	// ── Step 8: build TFOC rows (PHP check_in) ───────────────────────────────
	// kid transferred IN to owner's branch → owner FORWARDS money for remaining sessions
	// online royalty from to_bid, offline from to_batch venue, check to_category_master
	// attendance: date <= action_date AND bid != to_bid (prevCount)
	// sessions_done: all records within range (PHP had bid filter commented out)
	buildTFOCRows := func(transfers []transferRow) ([]map[string]any, float64, float64) {
		var total float64
		var gstCreditBalance float64
		rows := []map[string]any{}
		for _, t := range transfers {
			invoiceRows := invoiceRowsByInvoice[t.InvoiceID]
			if len(invoiceRows) == 0 {
				continue
			}
			for _, invData := range invoiceRows {
				if invData.ServiceSessions <= 0 {
					continue
				}
				actionDate := normalizeDateOnly(t.ActionDate)
				royaltyPct := onlineRoyaltyMap[t.ToBid]
				if t.ToCatMaster == 2 {
					if pct, ok := offlineRoyaltyMap[t.ToBatch]; ok {
						royaltyPct = pct
					}
				}
				royaltyAmount := invData.Amount * (float64(100-royaltyPct) / 100)
				perSession := royaltyAmount / float64(invData.ServiceSessions)
				attendanceRows := loadAttendanceRows(t.ContactID, invData.StartDate, actionDate)

				var prevCount int64
				branchCount := map[string]int{}
				branchOrder := []string{}
				for _, a := range attendanceRows {
					attDate := normalizeDateOnly(a.Date)
					if _, ok := branchCount[a.Branch]; !ok {
						branchOrder = append(branchOrder, a.Branch)
					}
					branchCount[a.Branch]++
					if attDate <= actionDate && a.Bid != t.ToBid {
						prevCount++
					}
				}
				// Keep parity with legacy PHP: do not clamp negative pending sessions.
				pendingSession := invData.ServiceSessions - prevCount
				forwardAmt := roundFloat(perSession*float64(pendingSession), 0)

				sessionsDone := []string{}
				for _, branch := range branchOrder {
					sessionsDone = append(sessionsDone, fmt.Sprintf("%s - %d", branch, branchCount[branch]))
				}

				// PHP check_in: only appends when not same owner
				if t.FromOwnerID == t.ToOwnerID {
					continue
				}
				total += forwardAmt
				gstCreditBalance += forwardAmt * 0.18
				rows = append(rows, map[string]any{
					"invoice_no":      invData.InvoiceNo,
					"name":            t.MemberName,
					"from_bid":        t.FromBidName,
					"to_bid":          t.ToBidName,
					"total_session":   invData.ServiceSessions,
					"amount":          roundFloat(invData.Amount, 2),
					"share":           roundFloat(royaltyAmount, 2),
					"pending_session": pendingSession,
					"forward":         forwardAmt,
					"same_owner":      0,
					"sessions_done":   sessionsDone,
				})
			}
		}
		return rows, total, roundFloat(gstCreditBalance, 2)
	}

	ttocRows, ttocTotal, ttocGSTCredit := buildTTOCRows(ttocTransfers)
	tfocRows, tfocTotal, tfocGSTCredit := buildTFOCRows(tfocTransfers)
	if ttocRows == nil {
		ttocRows = []map[string]any{}
	}
	if tfocRows == nil {
		tfocRows = []map[string]any{}
	}

	return &MemberTransferAnnexure{
		Title: "Member Transfer Annexure",
		Sections: []AnnexureSection{
			{
				Key:   "TTOC",
				Label: "Receivable Amount",
				Rows:  ttocRows,
				Totals: map[string]any{
					"total":              ttocTotal,
					"gst_credit_balance": ttocGSTCredit,
				},
			},
			{
				Key:   "TFOC",
				Label: "Forward-able Amount",
				Rows:  tfocRows,
				Totals: map[string]any{
					"total":              tfocTotal,
					"gst_credit_balance": tfocGSTCredit,
				},
			},
		},
	}, nil
}

// ── helpers ────────────────────────────────────────────────────────────────

// formatMonthYear converts "2025-03-01" → "Mar 25".
func formatMonthYear(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return ""
	}
	return t.Format("Jan 06")
}

func normalizeDateOnly(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 10 {
		return v[:10]
	}
	return v
}

// parseStoredParticulars parses the other_items JSON blob from franchise_invoice.
// Fixed keys (Royalty, TTOC, TFOC) are always present.
func parseStoredParticulars(raw string) map[string]*ParticularItem {
	fixedKeys := []string{
		"Royalty",
		"TTOC (Transfered to other center)",
		"TFOC (Transfered from other center)",
	}
	result := map[string]*ParticularItem{}
	for _, k := range fixedKeys {
		result[k] = &ParticularItem{}
	}
	if raw == "" {
		return result
	}
	var parsed map[string]map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return result
	}
	for k, v := range parsed {
		result[k] = &ParticularItem{
			Amount: toFloat(v["amount"]),
			HSN:    toString(v["hsn"]),
			IGST:   toFloat(v["igst"]),
			CGST:   toFloat(v["cgst"]),
			SGST:   toFloat(v["sgst"]),
		}
	}
	return result
}

// extractSubInvoiceItemMeta returns [label, hsn, gstAmount] from an other_items JSON blob.
func extractSubInvoiceItemMeta(raw string) [3]string {
	fallback := [3]string{"-", "-", "0.00"}
	if raw == "" {
		return fallback
	}
	var parsed map[string]map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return fallback
	}
	for k, item := range parsed {
		amount := toFloat(item["amount"])
		igst := toFloat(item["igst"])
		cgst := toFloat(item["cgst"])
		sgst := toFloat(item["sgst"])
		gst := amount*igst/100 + amount*cgst/100 + amount*sgst/100
		hsn := toString(item["hsn"])
		if hsn == "" {
			hsn = "-"
		}
		return [3]string{k, hsn, fmt.Sprintf("%.2f", gst)}
	}
	return fallback
}

func toFloat(v any) float64 {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		return 0
	}
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}
