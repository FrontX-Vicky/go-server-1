package reports

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"server_1/internal/core/httpx"
)

type Controller struct{ Repo *Repo }

type Report struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

// GET /api/v1/reports
func (ctl *Controller) List(c *gin.Context) {
	httpx.Fail(c, http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

// GET /api/v1/reports/:id
func (ctl *Controller) Show(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	opts := map[string]any{
		"start_date": c.Query("start_date"),
		"end_date":   c.Query("end_date"),
		"value":      c.Query("value"),
		"groupby":    c.Query("groupby"),
		"limit":      c.Query("limit"),
		"offset":     c.Query("offset"),
	}
	// TODO: parse orderby param if needed (array or string)
	queryOnly := c.Query("query_only") == "1"

	if queryOnly {
		q, err := ctl.Repo.BuildReportQuery(c.Request.Context(), id, opts)
		if err != nil {
			if err == sql.ErrNoRows {
				httpx.Fail(c, http.StatusNotFound, gin.H{"error": "not found"})
				return
			}
			httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		httpx.OK(c, gin.H{"query": q})
		return
	}

	// Run the query and return data
	data, query, err := ctl.Repo.RunReportQuery(c.Request.Context(), id, opts)
	if err != nil {
		if err == sql.ErrNoRows {
			httpx.Fail(c, http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error(), "query": query})
		return
	}

	// Also fetch structured columns metadata and include in the same response
	cols, cerr := ctl.Repo.GetStructuredColumns(c.Request.Context(), id)
	if cerr != nil && cerr != sql.ErrNoRows {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": cerr.Error()})
		return
	}
	if cerr == sql.ErrNoRows {
		cols = []StructuredColumn{}
	}

	httpx.OK(c, gin.H{"query": query, "data": data, "columns": cols})
}

// POST /api/v1/reports
func (ctl *Controller) Create(c *gin.Context) {
	httpx.Fail(c, http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

// PUT /api/v1/reports/:id
func (ctl *Controller) Update(c *gin.Context) {
	httpx.Fail(c, http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

// DELETE /api/v1/reports/:id
func (ctl *Controller) Delete(c *gin.Context) {
	httpx.Fail(c, http.StatusNotImplemented, gin.H{"error": "not implemented"})
}

// GET /api/v1/reports/:id/columns
func (ctl *Controller) Columns(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	cols, err := ctl.Repo.GetStructuredColumns(c.Request.Context(), id)
	if err != nil {
		if err == sql.ErrNoRows {
			httpx.OK(c, gin.H{"columns": []StructuredColumn{}})
			return
		}
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	httpx.OK(c, gin.H{"columns": cols})
}
