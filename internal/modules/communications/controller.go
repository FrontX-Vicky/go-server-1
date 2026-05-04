package communications

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"server_1/internal/core/httpx"
)

type Controller struct {
	Service *EmailService
}

func (ctl *Controller) CreateJob(c *gin.Context) {
	var req CreateEmailJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	resp, err := ctl.Service.CreateJob(c.Request.Context(), req)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	httpx.OK(c, gin.H{"data": resp})
}

func parseJobID(c *gin.Context) (int64, error) {
	return strconv.ParseInt(c.Param("jobId"), 10, 64)
}

func (ctl *Controller) GetJob(c *gin.Context) {
	jobID, err := parseJobID(c)
	if err != nil || jobID <= 0 {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "valid job id is required"})
		return
	}
	resp, err := ctl.Service.GetJob(c.Request.Context(), jobID)
	if err != nil {
		httpx.Fail(c, http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	httpx.OK(c, gin.H{"data": resp})
}

func (ctl *Controller) ListLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	resp, err := ctl.Service.ListLogs(c.Request.Context(), EmailJobListFilters{
		Status:        c.Query("status"),
		ModuleKey:     c.Query("module_key"),
		ReferenceType: c.Query("reference_type"),
		ReferenceID:   c.Query("reference_id"),
		Recipient:     c.Query("recipient"),
		Actor:         c.Query("actor"),
		DateFrom:      c.Query("date_from"),
		DateTo:        c.Query("date_to"),
		Limit:         limit,
		Offset:        offset,
	})
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	httpx.OK(c, gin.H{"data": resp})
}

func (ctl *Controller) RetryJob(c *gin.Context) {
	jobID, err := parseJobID(c)
	if err != nil || jobID <= 0 {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "valid job id is required"})
		return
	}
	resp, err := ctl.Service.RetryJob(c.Request.Context(), jobID)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	httpx.OK(c, gin.H{"data": resp})
}

func (ctl *Controller) GetReferenceJobs(c *gin.Context) {
	resp, err := ctl.Service.ListLogs(c.Request.Context(), EmailJobListFilters{
		ReferenceType: c.Param("referenceType"),
		ReferenceID:   c.Param("referenceId"),
		Limit:         200,
	})
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	httpx.OK(c, gin.H{"data": resp})
}
