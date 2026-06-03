package openapi

import (
	"net/http"
	"strconv"
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
	pageSize, err := parsePositiveQueryInt(c.Query("page_size"), 1000, 5000)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "page_size must be a positive integer up to 5000"})
		return
	}
	if pageSize == 1000 && c.Query("limit") != "" {
		pageSize, err = parsePositiveQueryInt(c.Query("limit"), 1000, 5000)
		if err != nil {
			httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "limit must be a positive integer up to 5000"})
			return
		}
	}

	page, err := parsePositiveQueryInt(c.Query("page"), 1, 0)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "page must be a positive integer"})
		return
	}

	rows, err := ctl.Repo.ActiveMembersRenewalRangePast(c.Request.Context(), endOfTwoMonthsAgo(time.Now()), page, pageSize)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rows)
}

func (ctl *Controller) GetCurrentData(c *gin.Context) {
	rows, err := ctl.Repo.GetCurrentData(c.Request.Context(), currentMonthToTodayRange(time.Now()))
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rows)
}

func (ctl *Controller) GetOldData(c *gin.Context) {
	rows, err := ctl.Repo.GetOldData(c.Request.Context(), oldMarketingRange(time.Now()))
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rows)
}

func (ctl *Controller) RevenueReport(c *gin.Context) {
	rows, err := ctl.Repo.RevenueReport(c.Request.Context())
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rows)
}

func (ctl *Controller) GetDemoFormResponseData(c *gin.Context) {
	rows, err := ctl.Repo.GetDemoFormResponseData(c.Request.Context())
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

func parsePositiveQueryInt(value string, fallback int, max int) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, strconv.ErrSyntax
	}
	if max > 0 && parsed > max {
		return 0, strconv.ErrRange
	}
	return parsed, nil
}
