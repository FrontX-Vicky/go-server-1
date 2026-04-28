package finance

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	coredb "server_1/internal/core/db"
)

type FranchiseInvoiceRepo struct {
	db1 *sql.DB
	db2 *sql.DB
}

func NewFranchiseInvoiceRepo() *FranchiseInvoiceRepo {
	return &FranchiseInvoiceRepo{
		db1: coredb.DB("DB1"),
		db2: coredb.DB("DB2"),
	}
}

// GetOwner fetches the owner name from branch_owner_master (DB1).
func (r *FranchiseInvoiceRepo) GetOwner(ctx context.Context, ownerID int64) (*FranchiseOwner, error) {
	row := r.db1.QueryRowContext(ctx,
		`SELECT id, owner_name FROM branch_owner_master WHERE id = ? AND park = 0 LIMIT 1`,
		ownerID,
	)
	var o FranchiseOwner
	if err := row.Scan(&o.ID, &o.OwnerName); err != nil {
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

// GetRoyaltyShare fetches total_sale and royalty from om_franchisee_combine_owner (DB2).
func (r *FranchiseInvoiceRepo) GetRoyaltyShare(ctx context.Context, ownerID int64, startDate, endDate string) (*RoyaltyShare, error) {
	row := r.db2.QueryRowContext(ctx,
		`SELECT COALESCE(total_sale,0), COALESCE(royalty,0), COALESCE(month,''), COALESCE(branch,'')
		 FROM om_franchisee_combine_owner
		 WHERE owner_id = ? AND start_date = ? AND end_date = ?
		 LIMIT 1`,
		ownerID, startDate, endDate,
	)
	var rs RoyaltyShare
	if err := row.Scan(&rs.TotalSale, &rs.Royalty, &rs.Month, &rs.Branch); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("royalty share not found for owner %d", ownerID)
		}
		return nil, err
	}
	return &rs, nil
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
	res, err := r.db1.ExecContext(ctx,
		`INSERT INTO franchise_invoice
		 (branch, owner_name_id, invoice, total_sale, royality, cgst, sgst, igst,
		  grant_total, other_items, month, month_year, start_date, end_date, invoice_date, park, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,NOW())`,
		req.Branch, req.OwnerID, req.Invoice, req.TotalSale,
		req.Royality, req.CGST, req.SGST, req.IGST,
		req.GrantTotal, req.OtherItems, req.Month,
		formatMonthYear(req.StartDate),
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
	res, err := r.db1.ExecContext(ctx,
		`INSERT INTO franchise_im_invoice
		 (parent_invoice_id, invoice, invoice_date, proforma, total_sale, royality,
		  cgst, sgst, igst, calculated_igst, grant_total, other_items, park, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,0,NOW())`,
		req.ParentInvoiceID, req.Invoice, req.InvoiceDate, req.Proforma,
		req.TotalSale, req.Royality, req.CGST, req.SGST, req.IGST,
		req.CalculatedIGST, req.GrantTotal, req.OtherItems,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}
// ListSubInvoices returns sub-invoices for a parent invoice (DB1).
// NOTE: Disabled - franchise_im_invoice table lacks parent_invoice_id column.
// TODO: Add database migration to add parent_invoice_id, then re-enable.
func (r *FranchiseInvoiceRepo) ListSubInvoices(ctx context.Context, parentInvoiceID int64) ([]SubInvoice, error) {
	return []SubInvoice{}, nil
}

// DeleteSubInvoice soft-deletes a sub-invoice (park=1) in DB1.
func (r *FranchiseInvoiceRepo) DeleteSubInvoice(ctx context.Context, subInvoiceID int64) error {
	_, err := r.db1.ExecContext(ctx,
		`UPDATE franchise_im_invoice SET park = 1 WHERE id = ? AND park = 0`,
		subInvoiceID,
	)
	return err
}

// CreateSalesInvoiceFromSub creates a sales invoice record from a sub-invoice (DB1).
// This mirrors the PHP frenchisee_sub_invoice_create_sales_invoice logic.
func (r *FranchiseInvoiceRepo) CreateSalesInvoiceFromSub(ctx context.Context, subInvoiceID int64) (int64, error) {
	// Fetch the sub-invoice first
	row := r.db1.QueryRowContext(ctx,
		`SELECT id, COALESCE(invoice,''), COALESCE(invoice_date,'0000-00-00'),
		        COALESCE(royality,0), COALESCE(cgst,0), COALESCE(sgst,0),
		        COALESCE(igst,0), COALESCE(grant_total,0), COALESCE(other_items,'')
		 FROM franchise_im_invoice WHERE id = ? AND park = 0`,
		subInvoiceID,
	)
	var (
		id          int64
		invoice     string
		invoiceDate string
		royality    float64
		cgst        float64
		sgst        float64
		igst        float64
		grantTotal  float64
		otherItems  string
	)
	if err := row.Scan(&id, &invoice, &invoiceDate, &royality, &cgst, &sgst, &igst, &grantTotal, &otherItems); err != nil {
		return 0, fmt.Errorf("sub-invoice %d not found: %w", subInvoiceID, err)
	}

	// Insert into sales_invoice_master
	res, err := r.db1.ExecContext(ctx,
		`INSERT INTO sales_invoice_master
		 (invoice_no, invoice_date, royality, cgst, sgst, igst, grant_total,
		  other_items, franchise_im_invoice_id, park, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,0,NOW())`,
		invoice, invoiceDate, royality, cgst, sgst, igst, grantTotal, otherItems, id,
	)
	if err != nil {
		return 0, err
	}
	salesInvoiceID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	// Link back to the sub-invoice
	if _, err := r.db1.ExecContext(ctx,
		`UPDATE franchise_im_invoice SET sales_invoice_id = ?, sales_invoice_no = ?
		 WHERE id = ?`,
		salesInvoiceID, invoice, id,
	); err != nil {
		return salesInvoiceID, err
	}

	return salesInvoiceID, nil
}

// GetMemberTransferAnnexure returns transfer annexure data from DB2.
func (r *FranchiseInvoiceRepo) GetMemberTransferAnnexure(ctx context.Context, ownerID int64, startDate, endDate string) (*MemberTransferAnnexure, error) {
	rows, err := r.db2.QueryContext(ctx,
		`SELECT direction, member_name, sessions, tds_amount, net_amount, forward_amount
		 FROM om_franchisee_member_transfer
		 WHERE owner_id = ? AND start_date = ? AND end_date = ?
		 ORDER BY direction, member_name`,
		ownerID, startDate, endDate,
	)
	if err != nil {
		// Table may not exist in all deployments; return empty rather than error.
		return &MemberTransferAnnexure{Title: "Member Transfer Annexure", Sections: []AnnexureSection{}}, nil
	}
	defer rows.Close()

	fromRows := []map[string]any{}
	toRows := []map[string]any{}
	var fromForward, fromAfterTDS, toForward, toAfterTDS float64

	for rows.Next() {
		var direction, memberName string
		var sessions int
		var tdsAmount, netAmount, forwardAmount float64
		if err := rows.Scan(&direction, &memberName, &sessions, &tdsAmount, &netAmount, &forwardAmount); err != nil {
			continue
		}
		entry := map[string]any{
			"member_name":    memberName,
			"sessions":       sessions,
			"tds_amount":     tdsAmount,
			"net_amount":     netAmount,
			"forward_amount": forwardAmount,
		}
		if strings.EqualFold(direction, "FROM") {
			fromRows = append(fromRows, entry)
			fromForward += forwardAmount
			fromAfterTDS += netAmount
		} else {
			toRows = append(toRows, entry)
			toForward += forwardAmount
			toAfterTDS += netAmount
		}
	}

	annexure := &MemberTransferAnnexure{
		Title: "Member Transfer Annexure",
		Sections: []AnnexureSection{
			{
				Key:   "FROM",
				Label: "Transferred From Other Center (TTOC)",
				Rows:  fromRows,
				Totals: map[string]any{
					"forward":   fromForward,
					"after_tds": fromAfterTDS,
				},
			},
			{
				Key:   "TO",
				Label: "Transferred To Other Center (TFOC)",
				Rows:  toRows,
				Totals: map[string]any{
					"forward":   toForward,
					"after_tds": toAfterTDS,
				},
			},
		},
	}
	return annexure, rows.Err()
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
