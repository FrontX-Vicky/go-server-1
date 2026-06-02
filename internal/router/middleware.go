package router

import (
    "context"
    "strings"
    "time"
    "github.com/gin-gonic/gin"
    "github.com/rs/zerolog/log"
)

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

        // Finance invoice-list runs a heavy aggregation over invoice_payment_franchisee_view.
        // Give this endpoint longer than the default request timeout.
        if strings.HasSuffix(c.Request.URL.Path, "/finance/franchise-invoice/invoice-list") {
            timeout = 90 * time.Second
        }

        ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
        defer cancel()
        c.Request = c.Request.WithContext(ctx)
        c.Next()
    }
}
