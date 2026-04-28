package finance

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"server_1/internal/core/httpx"
)

type FranchiseInvoiceController struct {
	Repo *FranchiseInvoiceRepo
}

// GetFranchiseInvoiceInit handles GET /finance/franchise-invoice/init
// Returns owner + existing invoice (or royalty+tax if no invoice exists).
func (ctl *FranchiseInvoiceController) GetFranchiseInvoiceInit(c *gin.Context) {
	ownerID, err := parseOwnerID(c)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	if startDate == "" || endDate == "" {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "start_date and end_date are required"})
		return
	}

	ctx := c.Request.Context()

	owner, err := ctl.Repo.GetOwner(ctx, ownerID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	monthYear := formatMonthYear(startDate)
	inv, err := ctl.Repo.GetFranchiseInvoice(ctx, ownerID, monthYear)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := FranchiseInvoiceInitResponse{Owner: owner}

	if inv != nil {
		resp.Invoice = inv
	} else {
		// No invoice yet — fetch royalty share and tax data
		rs, err := ctl.Repo.GetRoyaltyShare(ctx, ownerID, startDate, endDate)
		if err != nil {
			httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		resp.RoyaltyShare = rs

		td, err := ctl.Repo.GetTaxData(ctx, rs.Branch)
		if err != nil {
			httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		resp.TaxData = td
	}

	httpx.OK(c, gin.H{"data": resp})
}

// CreateFranchiseInvoice handles POST /finance/franchise-invoice
func (ctl *FranchiseInvoiceController) CreateFranchiseInvoice(c *gin.Context) {
	var req CreateFranchiseInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if req.OwnerID <= 0 {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "owner_id is required"})
		return
	}

	id, err := ctl.Repo.CreateFranchiseInvoice(c.Request.Context(), req)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	httpx.OK(c, gin.H{"data": id, "message": "Invoice created successfully"})
}

// UpdateFranchiseInvoice handles PUT /finance/franchise-invoice
func (ctl *FranchiseInvoiceController) UpdateFranchiseInvoice(c *gin.Context) {
	var req UpdateFranchiseInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if req.InvoiceID <= 0 || req.OwnerID <= 0 {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "invoice_id and owner_id are required"})
		return
	}

	if err := ctl.Repo.UpdateFranchiseInvoice(c.Request.Context(), req); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	httpx.OK(c, gin.H{"message": "Invoice updated successfully"})
}

// DeleteFranchiseInvoice handles DELETE /finance/franchise-invoice
func (ctl *FranchiseInvoiceController) DeleteFranchiseInvoice(c *gin.Context) {
	invoiceID, err := strconv.ParseInt(c.Query("invoice_id"), 10, 64)
	if err != nil || invoiceID <= 0 {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "valid invoice_id is required"})
		return
	}
	ownerID, err := strconv.ParseInt(c.Query("owner_id"), 10, 64)
	if err != nil || ownerID <= 0 {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "valid owner_id is required"})
		return
	}

	if err := ctl.Repo.DeleteFranchiseInvoice(c.Request.Context(), invoiceID, ownerID); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	httpx.OK(c, gin.H{"message": "Invoice deleted successfully"})
}

// CreateSubInvoice handles POST /finance/franchise-invoice/sub-invoice
func (ctl *FranchiseInvoiceController) CreateSubInvoice(c *gin.Context) {
	var req CreateSubInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if req.ParentInvoiceID <= 0 {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "parent_invoice_id is required"})
		return
	}

	id, err := ctl.Repo.CreateSubInvoice(c.Request.Context(), req)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	httpx.OK(c, gin.H{"data": id, "message": "Sub-invoice created successfully"})
}

// ListSubInvoices handles GET /finance/franchise-invoice/sub-invoices
func (ctl *FranchiseInvoiceController) ListSubInvoices(c *gin.Context) {
	parentID, err := strconv.ParseInt(c.Query("parent_invoice_id"), 10, 64)
	if err != nil || parentID <= 0 {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "valid parent_invoice_id is required"})
		return
	}

	list, err := ctl.Repo.ListSubInvoices(c.Request.Context(), parentID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	httpx.OK(c, gin.H{"data": list})
}

// DeleteSubInvoice handles DELETE /finance/franchise-invoice/sub-invoice
func (ctl *FranchiseInvoiceController) DeleteSubInvoice(c *gin.Context) {
	subID, err := strconv.ParseInt(c.Query("sub_invoice_id"), 10, 64)
	if err != nil || subID <= 0 {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "valid sub_invoice_id is required"})
		return
	}

	if err := ctl.Repo.DeleteSubInvoice(c.Request.Context(), subID); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	httpx.OK(c, gin.H{"message": "Sub-invoice deleted successfully"})
}

// CreateSalesInvoice handles POST /finance/franchise-invoice/sales-invoice
func (ctl *FranchiseInvoiceController) CreateSalesInvoice(c *gin.Context) {
	var body struct {
		SubInvoiceID int64 `json:"sub_invoice_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.SubInvoiceID <= 0 {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "valid sub_invoice_id is required"})
		return
	}

	salesID, err := ctl.Repo.CreateSalesInvoiceFromSub(c.Request.Context(), body.SubInvoiceID)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	httpx.OK(c, gin.H{"data": salesID, "message": "Sales invoice created successfully"})
}

// GetMemberTransferAnnexure handles GET /finance/franchise-invoice/annexure
func (ctl *FranchiseInvoiceController) GetMemberTransferAnnexure(c *gin.Context) {
	ownerID, err := parseOwnerID(c)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	annexure, err := ctl.Repo.GetMemberTransferAnnexure(c.Request.Context(), ownerID, startDate, endDate)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	httpx.OK(c, gin.H{"data": annexure})
}

// ── helper ────────────────────────────────────────────────────────────────

func parseOwnerID(c *gin.Context) (int64, error) {
	raw := c.Query("owner_id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("valid owner_id is required")
	}
	return id, nil
}

// ensure time import is used (formatMonthYear is in repo file; we import time here for consistency)
var _ = time.Now
