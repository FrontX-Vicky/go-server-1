package finance

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"server_1/internal/core/httpx"
)

type Controller struct{ Repo *Repo }

func (ctl *Controller) FranchiseeReport(c *gin.Context) {
	var req FranchiseeReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	if !IsSupportedReportID(req.ReportID) {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "unsupported report_id for finance franchisee report"})
		return
	}

	if strings.TrimSpace(req.StartDate) == "" || strings.TrimSpace(req.EndDate) == "" {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "start_date and end_date are required"})
		return
	}

	if req.Limit <= 0 {
		req.Limit = 100
	}
	if req.Limit > 500 {
		req.Limit = 500
	}
	if req.Offset < 0 {
		req.Offset = 0
	}

	if req.QueryOnly {
		query, err := ctl.Repo.BuildQuery(c.Request.Context(), req)
		if err != nil {
			httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		httpx.OK(c, gin.H{"query": query})
		return
	}

	resp, query, err := ctl.Repo.GetFranchiseeReport(c.Request.Context(), req)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Fail(c, http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error(), "query": query})
		return
	}

	httpx.OK(c, gin.H{"data": resp})
}
