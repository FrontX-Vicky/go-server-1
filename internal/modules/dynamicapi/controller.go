package dynamicapi

import (
	"crypto/subtle"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"server_1/internal/core/httpx"
)

type Controller struct {
	Repo   *Repo
	APIKey string
}

func NewController(repo *Repo, apiKey string) *Controller {
	return &Controller{
		Repo:   repo,
		APIKey: apiKey,
	}
}

func (ctl *Controller) Fetch(c *gin.Context) {
	if err := ctl.authorize(c); err != nil {
		httpx.Fail(c, http.StatusUnauthorized, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}

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

func (ctl *Controller) authorize(c *gin.Context) error {
	if ctl.APIKey == "" {
		return nil
	}
	headerKey := c.GetHeader("X-API-Key")
	if headerKey == "" {
		return errors.New("missing API key")
	}
	if subtle.ConstantTimeCompare([]byte(headerKey), []byte(ctl.APIKey)) != 1 {
		return errors.New("invalid API key")
	}
	return nil
}
