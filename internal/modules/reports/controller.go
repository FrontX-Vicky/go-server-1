package reports

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"

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
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	opts := map[string]any{
		"start_date": c.Query("start_date"),
		"end_date":   c.Query("end_date"),
		"value":      c.Query("value"),
		"groupby":    c.Query("groupby"),
		"limit":      c.Query("limit"),
		"offset":     c.Query("offset"),
	}
	// TODO: parse orderby param if needed (array or string)
	queryOnly := parseBoolString(c.Query("query_only"))

	ctl.handleReportRequest(c, id, opts, queryOnly)
}

// POST /api/v1/reports/:id
func (ctl *Controller) Execute(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	payload := map[string]any{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	opts := map[string]any{
		"start_date": stringFromPayload(payload, "start_date"),
		"end_date":   stringFromPayload(payload, "end_date"),
		"value":      stringFromPayload(payload, "value"),
		"groupby":    stringFromPayload(payload, "groupby"),
		"limit":      stringFromPayload(payload, "limit"),
		"offset":     stringFromPayload(payload, "offset"),
	}

	queryOnly := parseBoolValue(payload["query_only"])
	ctl.handleReportRequest(c, id, opts, queryOnly)
}

func (ctl *Controller) handleReportRequest(c *gin.Context, id int64, opts map[string]any, queryOnly bool) {

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

	payload := gin.H{
		// "query":   query,
		"data":    data,
		"columns": cols,
	}

	httpx.OK(c, payload)
	// httpx.OK(c, gin.H{"query": query, "data": data, "columns": cols, "project": payload})
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

func stringFromPayload(payload map[string]any, key string) string {
	if payload == nil {
		return ""
	}
	if val, ok := payload[key]; ok && val != nil {
		if str, ok := val.(string); ok {
			return str
		}
		return fmt.Sprint(val)
	}
	return ""
}

func parseBoolString(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	return value == "1" || value == "true" || value == "yes" || value == "y"
}

func parseBoolValue(val any) bool {
	switch v := val.(type) {
	case bool:
		return v
	case string:
		return parseBoolString(v)
	case float64:
		return v != 0
	case int:
		return v != 0
	case int64:
		return v != 0
	case uint:
		return v != 0
	default:
		if v == nil {
			return false
		}
		return parseBoolString(fmt.Sprint(v))
	}
}
