package router

import (
    "github.com/gin-gonic/gin"
    "server_1/internal/core/config"
    "server_1/internal/modules/test_items"
    // "server_1/internal/modules/reports"
)

func Build(cfg config.Config) *gin.Engine {
    r := gin.New()
    r.Use(gin.Recovery(), RequestLogger())

    base := r.Group(cfg.Server.BasePath)
    base.GET("/health", func(c *gin.Context) { c.String(200, "ok") })

    v1 := base.Group("/api/v1")
    test_items.MountRoutes(v1)
    // reports.MountRoutes(v1)
    return r
}
