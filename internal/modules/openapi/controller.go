package openapi

import (
	"net/http"
	"strings"
	"time"

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

func (ctl *Controller) ActiveMembersRenewalRangeCurrent(c *gin.Context) {
	month := defaultCurrentMonthRange(time.Now())
	if c.Query("start_date") != "" || c.Query("end_date") != "" {
		if c.Query("start_date") == "" || c.Query("end_date") == "" {
			httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "start_date and end_date are both required"})
			return
		}

		parsed, err := parseDateRange(c.Query("start_date"), c.Query("end_date"))
		if err != nil {
			httpx.Fail(c, http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		month = parsed
	}

	report := isTruthyQuery(c.Query("report"))
	rows, err := ctl.Repo.ActiveMembersRenewalRangeCurrent(c.Request.Context(), month, report)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rows)
}

func (ctl *Controller) ActiveMembersRenewalRangePast(c *gin.Context) {
	rows, err := ctl.Repo.ActiveMembersRenewalRangePast(c.Request.Context(), endOfTwoMonthsAgo(time.Now()))
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rows)
}

func isTruthyQuery(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
