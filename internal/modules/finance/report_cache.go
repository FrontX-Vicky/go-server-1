package finance

import (
\t"context"
\t"crypto/sha1"
\t"encoding/hex"
\t"encoding/json"
\t"fmt"
\t"os"
\t"strconv"
\t"strings"
\t"sync"
\t"time"

\t"github.com/redis/go-redis/v9"
)

const franchiseeReportCacheTTL = time.Hour

var (
\treportCacheOnce   sync.Once
\treportCacheClient *redis.Client
)

func getReportCacheClient() *redis.Client {
\treportCacheOnce.Do(func() {
\t\taddr := strings.TrimSpace(os.Getenv("REDIS_ADDR"))
\t\tif addr == "" {
\t\t\taddr = "127.0.0.1:6379"
\t\t}

\t\tdb := 0
\t\tif raw := strings.TrimSpace(os.Getenv("REDIS_DB")); raw != "" {
\t\t\tif parsed, err := strconv.Atoi(raw); err == nil {
\t\t\t\tdb = parsed
\t\t\t}
\t\t}

\t\treportCacheClient = redis.NewClient(&redis.Options{
\t\t\tAddr:     addr,
\t\t\tPassword: os.Getenv("REDIS_PASSWORD"),
\t\t\tDB:       db,
\t\t})
\t})

\treturn reportCacheClient
}

func franchiseeReportCacheKey(req FranchiseeReportRequest) string {
\traw := fmt.Sprintf(
\t\t"report_id=%d|start=%s|end=%s|group_by=%s|limit=%d|offset=%d|order=%s",
\t\treq.ReportID,
\t\tstrings.TrimSpace(req.StartDate),
\t\tstrings.TrimSpace(req.EndDate),
\t\tstrings.TrimSpace(req.GroupBy),
\t\treq.Limit,
\t\treq.Offset,
\t\tcanonicalOrderBy(req.OrderBy),
\t)
\thash := sha1.Sum([]byte(raw))
\treturn "finance:franchisee-report:" + hex.EncodeToString(hash[:])
}

func canonicalOrderBy(orderBy []OrderBy) string {
\tif len(orderBy) == 0 {
\t\treturn ""
\t}

\tparts := make([]string, 0, len(orderBy))
\tfor _, order := range orderBy {
\t\tcol := strings.TrimSpace(order.Column)
\t\tdir := strings.ToUpper(strings.TrimSpace(order.Direction))
\t\tif dir != "DESC" {
\t\t\tdir = "ASC"
\t\t}
\t\tif col == "" {
\t\t\tcontinue
\t\t}
\t\tparts = append(parts, col+":"+dir)
\t}

\treturn strings.Join(parts, ",")
}

func getCachedFranchiseeReport(ctx context.Context, req FranchiseeReportRequest) (*FranchiseeReportResponse, bool) {
\tclient := getReportCacheClient()
\tif client == nil {
\t\treturn nil, false
\t}

\tkey := franchiseeReportCacheKey(req)
\tencoded, err := client.Get(ctx, key).Result()
\tif err != nil {
\t\treturn nil, false
\t}

\tvar resp FranchiseeReportResponse
\tif err := json.Unmarshal([]byte(encoded), &resp); err != nil {
\t\treturn nil, false
\t}

\treturn &resp, true
}

func setCachedFranchiseeReport(ctx context.Context, req FranchiseeReportRequest, resp *FranchiseeReportResponse) {
\tif resp == nil {
\t\treturn
\t}

\tclient := getReportCacheClient()
\tif client == nil {
\t\treturn
\t}

\tencoded, err := json.Marshal(resp)
\tif err != nil {
\t\treturn
\t}

\tkey := franchiseeReportCacheKey(req)
\t_ = client.Set(ctx, key, encoded, franchiseeReportCacheTTL).Err()
}
