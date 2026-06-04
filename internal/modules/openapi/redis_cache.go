package openapi

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
	openapiCacheTTL         = 24 * time.Hour
	openapiCacheRefreshHits  = 5
	openapiCacheOpTimeout    = 200 * time.Millisecond
	openapiCacheInitTimeout  = 300 * time.Millisecond
)

var (
	openapiRedisOnce   sync.Once
	openapiRedisClient  *redis.Client
)

func getOpenAPIRedisClient() *redis.Client {
	openapiRedisOnce.Do(func() {
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
			DialTimeout:  openapiCacheOpTimeout,
			ReadTimeout:  openapiCacheOpTimeout,
			WriteTimeout: openapiCacheOpTimeout,
			PoolTimeout:  openapiCacheOpTimeout,
		})

		pingCtx, cancel := context.WithTimeout(context.Background(), openapiCacheInitTimeout)
		defer cancel()
		if err := client.Ping(pingCtx).Err(); err != nil {
			_ = client.Close()
			openapiRedisClient = nil
			return
		}

		openapiRedisClient = client
	})

	return openapiRedisClient
}

func openapiCacheKey(parts ...any) string {
	raw := fmt.Sprint(parts...)
	hash := sha1.Sum([]byte(raw))
	return "openapi:" + hex.EncodeToString(hash[:])
}

func getCachedOpenAPIJSON(ctx context.Context, key string, dest any) bool {
	client := getOpenAPIRedisClient()
	if client == nil {
		return false
	}

	encoded, err := client.Get(ctx, key).Result()
	if err != nil {
		return false
	}

	return json.Unmarshal([]byte(encoded), dest) == nil
}

func setCachedOpenAPIJSON(ctx context.Context, key string, value any, ttl time.Duration) {
	client := getOpenAPIRedisClient()
	if client == nil {
		return
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return
	}
	_ = client.Set(ctx, key, encoded, ttl).Err()
}

func refreshHitCount(ctx context.Context, key string, ttl time.Duration, refreshAfter int) (int, error) {
	client := getOpenAPIRedisClient()
	if client == nil {
		return 0, nil
	}

	count, err := client.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if count == 1 {
		_ = client.Expire(ctx, key, ttl).Err()
	}
	if refreshAfter > 0 && count >= int64(refreshAfter) {
		return int(count), nil
	}
	return int(count), nil
}

func resetHitCount(ctx context.Context, key string) {
	client := getOpenAPIRedisClient()
	if client == nil {
		return
	}
	_ = client.Del(ctx, key).Err()
}

