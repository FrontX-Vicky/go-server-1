package router

import (
    "context"
    "time"
    "github.com/gin-gonic/gin"
    "github.com/rs/zerolog/log"
)

const timeoutOverrideKey = "request-timeout-override"

func RequestLogger() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        c.Next()
        log.Info().
            Str("method", c.Request.Method).
            Str("path", c.Request.URL.Path).
            Int("status", c.Writer.Status()).
            Dur("dur", time.Since(start)).
            Msg("req")
    }
}

// WithTimeout injects a deadline into the request context so long-running
// handlers (e.g., slow SQL) get canceled and return instead of tying up connections.
func WithTimeout(d time.Duration) gin.HandlerFunc {
    return func(c *gin.Context) {
        timeout := d
        if v, ok := c.Get(timeoutOverrideKey); ok {
            if override, ok := v.(time.Duration); ok && override > 0 {
                timeout = override
            }
        }

        ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
        defer cancel()
        c.Request = c.Request.WithContext(ctx)
        c.Next()
    }
}

// WithRequestTimeoutOverride sets a per-request timeout that WithTimeout will honor.
func WithRequestTimeoutOverride(d time.Duration) gin.HandlerFunc {
    return func(c *gin.Context) {
        if d > 0 {
            c.Set(timeoutOverrideKey, d)
        }
        c.Next()
    }
}
