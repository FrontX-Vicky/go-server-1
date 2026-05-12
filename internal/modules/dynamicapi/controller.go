package dynamicapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"server_1/internal/core/httpx"
)

type Controller struct {
	Repo *Repo
}

func NewController(repo *Repo) *Controller {
	return &Controller{
		Repo: repo,
	}
}

func (ctl *Controller) Fetch(c *gin.Context) {
	var req QueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}

	if err := req.Normalize(); err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}

	result, err := ctl.Repo.Fetch(c.Request.Context(), &req)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}

	response := gin.H{
		"data": result.Rows,
		"page": gin.H{
			"number": req.Page.Number,
			"size":   req.Page.Size,
		},
	}
	if req.IncludeTotal && result.Total != nil {
		response["total"] = *result.Total
	}
	if len(req.Meta) > 0 {
		response["meta"] = req.Meta
	}

	httpx.OK(c, response)
}
