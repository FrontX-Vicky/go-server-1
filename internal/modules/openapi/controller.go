package openapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"server_1/internal/core/httpx"
)

type Controller struct {
	Repo *Repo
}

func NewController(repo *Repo) *Controller {
	return &Controller{Repo: repo}
}

func (ctl *Controller) Health(c *gin.Context) {
	httpx.OK(c, gin.H{"ok": true})
}

func (ctl *Controller) InquiryDemoFollowup(c *gin.Context) {
	rows, err := ctl.Repo.InquiryDemoFollowup(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rows)
}
