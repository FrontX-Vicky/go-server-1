package finance

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	franchiseeReportCacheTTL = time.Hour
	reportCacheOpTimeout     = 200 * time.Millisecond
	reportCacheInitTimeout   = 300 * time.Millisecond
)

var (
	reportCacheOnce   sync.Once
	reportCacheClient *redis.Client
)

func getReportCacheClient() *redis.Client {
	reportCacheOnce.Do(func() {
		addr := strings.TrimSpace(os.Getenv("REDIS_ADDR"))
		if addr == "" {
			addr = "127.0.0.1:6379"
		}

		db := 0
		if raw := strings.TrimSpace(os.Getenv("REDIS_DB")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil {
				db = parsed
			}
		}

		client := redis.NewClient(&redis.Options{
			Addr:         addr,
			Password:     os.Getenv("REDIS_PASSWORD"),
			DB:           db,
			DialTimeout:  reportCacheOpTimeout,
			ReadTimeout:  reportCacheOpTimeout,
			WriteTimeout: reportCacheOpTimeout,
			PoolTimeout:  reportCacheOpTimeout,
		})

		pingCtx, cancel := context.WithTimeout(context.Background(), reportCacheInitTimeout)
		defer cancel()
		if err := client.Ping(pingCtx).Err(); err != nil {
			_ = client.Close()
			reportCacheClient = nil
			return
		}

		reportCacheClient = client
	})

	return reportCacheClient
}

func franchiseeReportCacheKey(req FranchiseeReportRequest) string {
	raw := fmt.Sprintf(
		"report_id=%d|start=%s|end=%s|group_by=%s|limit=%d|offset=%d|order=%s",
		req.ReportID,
		strings.TrimSpace(req.StartDate),
		strings.TrimSpace(req.EndDate),
		strings.TrimSpace(req.GroupBy),
		req.Limit,
		req.Offset,
		canonicalOrderBy(req.OrderBy),
	)
	hash := sha1.Sum([]byte(raw))
	return "finance:franchisee-report:" + hex.EncodeToString(hash[:])
}

func canonicalOrderBy(orderBy []OrderBy) string {
	if len(orderBy) == 0 {
		return ""
	}

	parts := make([]string, 0, len(orderBy))
	for _, order := range orderBy {
		col := strings.TrimSpace(order.Column)
		dir := strings.ToUpper(strings.TrimSpace(order.Direction))
		if dir != "DESC" {
			dir = "ASC"
		}
		if col == "" {
			continue
		}
		parts = append(parts, col+":"+dir)
	}

	return strings.Join(parts, ",")
}

func getCachedFranchiseeReport(ctx context.Context, req FranchiseeReportRequest) (*FranchiseeReportResponse, bool) {
	client := getReportCacheClient()
	if client == nil {
		return nil, false
	}

	key := franchiseeReportCacheKey(req)
	encoded, err := client.Get(ctx, key).Result()
	if err != nil {
		return nil, false
	}

	var resp FranchiseeReportResponse
	if err := json.Unmarshal([]byte(encoded), &resp); err != nil {
		return nil, false
	}

	return &resp, true
}

func setCachedFranchiseeReport(ctx context.Context, req FranchiseeReportRequest, resp *FranchiseeReportResponse) {
	if resp == nil {
		return
	}

	client := getReportCacheClient()
	if client == nil {
		return
	}

	encoded, err := json.Marshal(resp)
	if err != nil {
		return
	}

	key := franchiseeReportCacheKey(req)
	_ = client.Set(ctx, key, encoded, franchiseeReportCacheTTL).Err()
}
