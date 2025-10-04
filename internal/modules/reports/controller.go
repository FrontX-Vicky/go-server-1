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
    // optional params used for dynamic reports
    startDate := c.Query("start_date")
    endDate := c.Query("end_date")
    value := c.Query("value")
    queryOnly := c.Query("query_only") == "1"

    // If user asked for query only, build and return the SQL string
    if queryOnly {

        q, err := ctl.Repo.BuildReportQuery(c.Request.Context(), id, startDate, endDate, value)
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

    meta, err := ctl.Repo.Get(c.Request.Context(), id)
    if err != nil {
        if err == sql.ErrNoRows {
            httpx.Fail(c, http.StatusNotFound, gin.H{"error": "not found"})
            return
        }
        httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    httpx.OK(c, gin.H{"data": meta})
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
