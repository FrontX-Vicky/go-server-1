package expense

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"server_1/internal/core/httpx"
)

// Controller handles HTTP requests for the expense module.
type Controller struct {
	Repo *Repo
}

// GET /api/v1/expense
// Accepts query params: start_date, end_date, search, limit, offset, query_only
func (ctl *Controller) List(c *gin.Context) {
	req := ExpenseListRequest{
		StartDate: c.Query("start_date"),
		EndDate:   c.Query("end_date"),
		Search:    c.Query("search"),
	}

	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			req.Limit = n
		}
	}
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			req.Offset = n
		}
	}
	req.QueryOnly = parseBool(c.Query("query_only"))

	ctl.handle(c, req)
}

// POST /api/v1/expense
// Accepts the same fields as GET, but via JSON body (mirrors finance/reports pattern).
func (ctl *Controller) Execute(c *gin.Context) {
	var req ExpenseListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	ctl.handle(c, req)
}

func (ctl *Controller) handle(c *gin.Context, req ExpenseListRequest) {
	if req.QueryOnly {
		q, err := ctl.Repo.BuildQuery(c.Request.Context(), req)
		if err != nil {
			httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		httpx.OK(c, gin.H{"query": q})
		return
	}

	resp, query, err := ctl.Repo.GetExpenseList(c.Request.Context(), req)
	if err != nil {
		if err == sql.ErrNoRows {
			httpx.OK(c, gin.H{
				"rows":       []any{},
				"pagination": ExpensePagination{},
			})
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error(), "query": query})
		return
	}

	httpx.OK(c, gin.H{"data": resp})
}

func parseBool(s string) bool {
	return s == "1" || s == "true" || s == "yes"
}
