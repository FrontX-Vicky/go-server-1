//go:build ignore

// Template command for tailing Redis Stream CDC payloads.
// Copy this file, tweak the defaults below (or override with env vars),
// then run with: go run ./scripts/templates/redis_stream_subscriber.go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"server_1/internal/events"
)

// Reuse shared CDC payload structures from internal/events.
type (
	Change = events.Change
	Event  = events.Event
)

// ===================== CONFIG (edit defaults or set env) =====================

const (
	defaultRedisAddr            = "127.0.0.1:6379"
	defaultRedisPass            = ""
	defaultRedisDB              = 0
	defaultStreamName           = "binlog:all"
	defaultGroupName            = "dashboard"
	defaultConsumerID           = "dash-1"
	defaultStartID              = "$"
	defaultPrettyPrint          = false
	defaultDebugLogs            = false
	defaultFilterEnvSeparator   = ","
	defaultFiltersCaseSensitive = false
)

var (
	redisAddr = envString("REDIS_ADDR", defaultRedisAddr)
	redisPass = envString("REDIS_PASSWORD", defaultRedisPass)
	redisDB   = envInt("REDIS_DB", defaultRedisDB)

	streamName = envString("REDIS_STREAM_NAME", defaultStreamName)
	groupName  = envString("REDIS_STREAM_GROUP", defaultGroupName)
	consumerID = envString("REDIS_STREAM_CONSUMER", defaultConsumerID)
	startID    = envString("REDIS_STREAM_START_ID", defaultStartID)

	prettyPrint    = envBool("REDIS_STREAM_PRETTY_PRINT", defaultPrettyPrint)
	enableDebugLog = envBool("REDIS_STREAM_DEBUG", defaultDebugLogs)

	filterDBs       = envList("REDIS_FILTER_DBS", nil)
	filterTables    = envList("REDIS_FILTER_TABLES", nil)
	filterIDs       = envList("REDIS_FILTER_IDS", nil)
	filterOps       = envList("REDIS_FILTER_OPS", nil)        // "create","update","delete"
	filterChangeAny = envList("REDIS_FILTER_CHANGE_ANY", nil) // require at least one of these columns
	filterChangeAll = envList("REDIS_FILTER_CHANGE_ALL", nil) // require all of these columns
)

// ===================== REDIS CONSUMER =====================

type Consumer struct{ R *redis.Client }

func NewConsumer(addr, pass string, db int) *Consumer {
	return &Consumer{
		R: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: pass,
			DB:       db,
		}),
	}
}

// EnsureGroup creates the consumer group if it does not already exist.
// startID: "$" (only new entries) or "0-0" (replay history).
func (c *Consumer) EnsureGroup(ctx context.Context, stream, group, startID string) error {
	err := c.R.XGroupCreateMkStream(ctx, stream, group, startID).Err()
	if err != nil {
		// go-redis/v9 returns BUSYGROUP when the group already exists.
		if strings.Contains(strings.ToUpper(err.Error()), "BUSYGROUP") {
			return nil
		}
		return err
	}
	return nil
}

func getPayload(vals map[string]any) (string, bool) {
	v, ok := vals["payload"]
	if !ok || v == nil {
		return "", false
	}
	switch t := v.(type) {
	case string:
		return t, true
	case []byte:
		return string(t), true
	default:
		return fmt.Sprintf("%v", t), true
	}
}

func (c *Consumer) Run(ctx context.Context, stream, group, consumer string, handle func(string) error) error {
	const (
		block      = 5 * time.Second
		count      = 128
		claimAfter = 60 * time.Second
	)

	claimStale := func() {
		pending, err := c.R.XPendingExt(ctx, &redis.XPendingExtArgs{
			Stream: stream,
			Group:  group,
			Start:  "-",
			End:    "+",
			Count:  500,
			Idle:   claimAfter,
		}).Result()
		if err != nil || len(pending) == 0 {
			return
		}
		ids := make([]string, 0, len(pending))
		for _, p := range pending {
			ids = append(ids, p.ID)
		}
		msgs, err := c.R.XClaim(ctx, &redis.XClaimArgs{
			Stream:   stream,
			Group:    group,
			Consumer: consumer,
			MinIdle:  claimAfter,
			Messages: ids,
		}).Result()
		if err != nil {
			if enableDebugLog {
				log.Printf("[claim] XClaim err: %v", err)
			}
			return
		}
		for _, m := range msgs {
			if raw, ok := getPayload(m.Values); ok && raw != "" {
				if err := handle(raw); err == nil {
					_ = c.R.XAck(ctx, stream, group, m.ID).Err()
				} else if enableDebugLog {
					log.Printf("[claim] handler error for %s: %v", m.ID, err)
				}
			} else {
				if enableDebugLog {
					log.Printf("[claim] missing/empty payload; fields: %#v", m.Values)
				}
				_ = c.R.XAck(ctx, stream, group, m.ID).Err()
			}
		}
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			claimStale()
		default:
			res, err := c.R.XReadGroup(ctx, &redis.XReadGroupArgs{
				Group:    group,
				Consumer: consumer,
				Streams:  []string{stream, ">"},
				Count:    count,
				Block:    block,
			}).Result()
			if err == redis.Nil {
				continue
			}
			if err != nil {
				if enableDebugLog {
					log.Printf("[read] XReadGroup err: %v", err)
				}
				time.Sleep(time.Second)
				continue
			}
			for _, s := range res {
				for _, m := range s.Messages {
					if raw, ok := getPayload(m.Values); ok && raw != "" {
						if err := handle(raw); err == nil {
							_ = c.R.XAck(ctx, stream, group, m.ID).Err()
						} else if enableDebugLog {
							log.Printf("[read] handler error for %s: %v", m.ID, err)
						}
						continue
					}
					if enableDebugLog {
						log.Printf("[warn] missing/empty payload; fields: %#v", m.Values)
					}
					_ = c.R.XAck(ctx, stream, group, m.ID).Err()
				}
			}
		}
	}
}

// ===================== FILTER UTILS =====================

type strset map[string]struct{}

func toSet(list []string, normalizeLower bool) strset {
	if len(list) == 0 {
		return nil
	}
	out := make(strset, len(list))
	for _, v := range list {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if normalizeLower {
			v = strings.ToLower(v)
		}
		out[v] = struct{}{}
	}
	return out
}

func inSet(s strset, v string) bool {
	if s == nil {
		return true
	}
	_, ok := s[v]
	return ok
}

func rowKeyToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	default:
		return fmt.Sprintf("%v", t)
	}
}

func hasAnyColumns(changes []Change, cols strset) bool {
	if cols == nil {
		return true
	}
	for _, c := range changes {
		if _, ok := cols[c.Column]; ok {
			return true
		}
	}
	return false
}

func hasAllColumns(changes []Change, cols strset) bool {
	if cols == nil || len(cols) == 0 {
		return true
	}
	seen := map[string]bool{}
	for _, c := range changes {
		seen[c.Column] = true
	}
	for col := range cols {
		if !seen[col] {
			return false
		}
	}
	return true
}

// ===================== MAIN =====================

func main() {
	_ = godotenv.Load(".env")

	log.Printf("subscriber start | redis=%s db=%d stream=%s group=%s consumer=%s start=%s",
		redisAddr, redisDB, streamName, groupName, consumerID, startID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cons := NewConsumer(redisAddr, redisPass, redisDB)

	if err := cons.EnsureGroup(ctx, streamName, groupName, startID); err != nil {
		log.Fatalf("EnsureGroup failed: %v", err)
	}

	dbSet := toSet(filterDBs, defaultFiltersCaseSensitive)
	tableSet := toSet(filterTables, defaultFiltersCaseSensitive)
	idSet := toSet(filterIDs, defaultFiltersCaseSensitive)
	opSet := toSet(filterOps, true)
	changeAny := toSet(filterChangeAny, defaultFiltersCaseSensitive)
	changeAll := toSet(filterChangeAll, defaultFiltersCaseSensitive)

	handle := func(raw string) error {
		var ev Event
		dec := json.NewDecoder(strings.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&ev); err != nil {
			return fmt.Errorf("json decode: %w", err)
		}

		if !inSet(dbSet, ev.DB) {
			return nil
		}
		if !inSet(tableSet, ev.Table) {
			return nil
		}
		if !inSet(idSet, rowKeyToString(ev.RowKey)) {
			return nil
		}
		if !inSet(opSet, strings.ToLower(ev.Op)) {
			return nil
		}
		if !hasAnyColumns(ev.Changes, changeAny) {
			return nil
		}
		if !hasAllColumns(ev.Changes, changeAll) {
			return nil
		}

		if prettyPrint {
			var obj map[string]any
			_ = json.Unmarshal([]byte(raw), &obj)
			b, _ := json.MarshalIndent(obj, "", "  ")
			fmt.Println(string(b))
		} else {
			fmt.Println(raw)
		}
		return nil
	}

	go func() {
		if err := cons.Run(ctx, streamName, groupName, consumerID, handle); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatalf("consumer exited: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	log.Println("shutting down.")
	cancel()
	time.Sleep(300 * time.Millisecond)
}

// ===================== ENV HELPERS =====================

func envString(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes", "y", "on":
			return true
		case "0", "false", "no", "n", "off":
			return false
		}
	}
	return fallback
}

func envList(key string, fallback []string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		if fallback == nil {
			return []string{}
		}
		return fallback
	}
	parts := strings.Split(raw, defaultFilterEnvSeparator)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
