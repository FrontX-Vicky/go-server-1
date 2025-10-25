package leads

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"server_1/internal/core/httpx"
)

type Controller struct{ Repo *Repo }

// RangeRequest kept for reference:
// type RangeRequest struct {
// 	StartDate string `json:"start_date"`
// 	EndDate   string `json:"end_date"`
// }

type FilterRequest struct {
	Filter [][]string `json:"filter"`
}

// POST /api/v1/leads/query
func (ctl *Controller) Query(c *gin.Context) {
	var req FilterRequest
	if c.Request.ContentLength == 0 {
		req.Filter = nil
	} else {
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}
	}

	data, err := ctl.Repo.FetchByFilters(c.Request.Context(), req.Filter)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	httpx.OK(c, gin.H{"data": data})
}

// POST /api/v1/leads/top-summary
func (ctl *Controller) TopSummary(c *gin.Context) {
	req, ok := bindFilterRequest(c)
	if !ok {
		return
	}

	summary, err := ctl.Repo.BuildTopSummary(c.Request.Context(), req.Filter)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if summary == nil {
		httpx.OK(c, gin.H{"data": gin.H{}})
		return
	}

	httpx.OK(c, gin.H{"data": summary})
}

// POST /api/v1/leads/source-breakdown
func (ctl *Controller) SourceBreakdown(c *gin.Context) {
	req, ok := bindFilterRequest(c)
	if !ok {
		return
	}

	breakdown, err := ctl.Repo.BuildSourceBreakdown(c.Request.Context(), req.Filter)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	httpx.OK(c, gin.H{"data": breakdown})
}

// POST /api/v1/leads/center-performance
func (ctl *Controller) CenterPerformance(c *gin.Context) {
	req, ok := bindFilterRequest(c)
	if !ok {
		return
	}

	perf, err := ctl.Repo.BuildCenterPerformance(c.Request.Context(), req.Filter)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	httpx.OK(c, gin.H{"data": perf})
}

func bindFilterRequest(c *gin.Context) (FilterRequest, bool) {
	var req FilterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return FilterRequest{}, false
	}
	if len(req.Filter) == 0 {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "filter is required"})
		return FilterRequest{}, false
	}
	for i, f := range req.Filter {
		if len(f) < 3 {
			httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "each filter item must have field, operator, value", "index": i})
			return FilterRequest{}, false
		}
	}
	return req, true
}
