package export

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"server_1/internal/core/httpx"
	"server_1/internal/modules/dynamicapi"
)

type Controller struct {
	Repo *dynamicapi.Repo
}

func NewController(repo *dynamicapi.Repo) *Controller {
	return &Controller{Repo: repo}
}

func (ctl *Controller) Export(c *gin.Context) {
	format := strings.ToLower(strings.TrimSpace(c.DefaultQuery("format", "csv")))
	if format != "" && format != "csv" {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "only csv format is supported"})
		return
	}

	exportAll := parseBoolQuery(c.Query("exportAll"))
	includeBOM := parseBoolQuery(c.Query("bom"))

	var req dynamicapi.QueryRequest
	switch {
	case c.Request.ContentLength > 0:
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}
	case c.Query("payload") != "":
		if err := json.Unmarshal([]byte(c.Query("payload")), &req); err != nil {
			httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}
	default:
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "payload is required"})
		return
	}

	if err := req.Normalize(); err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rows, err := ctl.collectRows(c.Request.Context(), &req, exportAll)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	columns := determineColumns(&req, rows)
	filename := buildFilename(req.Table)

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)

	if includeBOM {
		if _, err := c.Writer.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
			return
		}
	}

	writer := csv.NewWriter(c.Writer)
	if len(columns) > 0 {
		if err := writer.Write(columns); err != nil {
			writer.Flush()
			return
		}
	}

	for _, row := range rows {
		record := make([]string, len(columns))
		for i, col := range columns {
			record[i] = formatCSVValue(row[col])
		}
		if err := writer.Write(record); err != nil {
			writer.Flush()
			return
		}
	}
	writer.Flush()
}

func (ctl *Controller) collectRows(ctx context.Context, req *dynamicapi.QueryRequest, exportAll bool) ([]map[string]any, error) {
	base := *req
	base.IncludeTotal = false
	if !exportAll {
		result, err := ctl.Repo.Fetch(ctx, &base)
		if err != nil {
			return nil, err
		}
		return result.Rows, nil
	}

	rows := make([]map[string]any, 0)
	pageNumber := base.Page.Number
	if pageNumber <= 0 {
		pageNumber = 1
	}
	for {
		current := base
		current.Page.Number = pageNumber

		result, err := ctl.Repo.Fetch(ctx, &current)
		if err != nil {
			return nil, err
		}

		if len(result.Rows) == 0 {
			break
		}

		rows = append(rows, result.Rows...)
		if len(result.Rows) < current.Page.Size {
			break
		}

		pageNumber++
	}

	return rows, nil
}

func determineColumns(req *dynamicapi.QueryRequest, rows []map[string]any) []string {
	if len(req.Select) > 0 {
		columns := make([]string, 0, len(req.Select))
		seen := make(map[string]struct{}, len(req.Select))
		for _, field := range req.Select {
			name := normalizeColumnName(field)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			columns = append(columns, name)
			seen[name] = struct{}{}
		}
		if len(columns) > 0 {
			return columns
		}
	}

	if len(rows) == 0 {
		return nil
	}

	keys := make([]string, 0, len(rows[0]))
	for key := range rows[0] {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func parseBoolQuery(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func normalizeColumnName(field string) string {
	field = strings.TrimSpace(field)
	if field == "" {
		return field
	}
	if idx := strings.LastIndex(field, "."); idx >= 0 {
		return field[idx+1:]
	}
	return field
}

var filenameSanitizer = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func buildFilename(table string) string {
	base := strings.TrimSpace(table)
	if base == "" {
		base = "export"
	}
	base = normalizeColumnName(base)
	base = filenameSanitizer.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "export"
	}
	timestamp := time.Now().UTC().Format("20060102-150405")
	return fmt.Sprintf("%s-%s.csv", base, timestamp)
}

func formatCSVValue(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case time.Time:
		if v.IsZero() {
			return ""
		}
		return v.UTC().Format(time.RFC3339)
	case fmt.Stringer:
		return v.String()
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(v)
	}
}
