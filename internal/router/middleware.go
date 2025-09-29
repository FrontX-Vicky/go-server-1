package router

import (
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
