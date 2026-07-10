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
	limit, offset, usePagination, err := parseOptionalPagination(c)
	if err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rows, err := ctl.Repo.InquiryDemoFollowup(c.Request.Context(), limit, offset, usePagination)
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
	var filter *dateRange
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
		filter = &parsed
	}

	rows, err := ctl.Repo.ActiveMembersRenewalRangePast(c.Request.Context(), endOfTwoMonthsAgo(time.Now()), filter)
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

func parseOptionalPagination(c *gin.Context) (limit int, offset int, usePagination bool, err error) {
	rawLimit := strings.TrimSpace(c.Query("limit"))
	rawOffset := strings.TrimSpace(c.Query("offset"))
	if rawLimit == "" && rawOffset == "" {
		return 0, 0, false, nil
	}

	limit = 10000
	offset = 0

	if rawLimit != "" {
		parsed, parseErr := strconv.Atoi(rawLimit)
		if parseErr != nil || parsed <= 0 {
			return 0, 0, false, errInvalidQueryValue("limit")
		}
		if parsed > 10000 {
			return 0, 0, false, errQueryTooLarge("limit", 10000)
		}
		limit = parsed
	}

	if rawOffset != "" {
		parsed, parseErr := strconv.Atoi(rawOffset)
		if parseErr != nil || parsed < 0 {
			return 0, 0, false, errInvalidQueryValue("offset")
		}
		offset = parsed
	}

	return limit, offset, true, nil
}

func errInvalidQueryValue(name string) error {
	return &queryParamError{message: name + " must be a valid integer"}
}

func errQueryTooLarge(name string, max int) error {
	return &queryParamError{message: name + " must be less than or equal to " + strconv.Itoa(max)}
}

type queryParamError struct {
	message string
}

func (e *queryParamError) Error() string {
	return e.message
}
