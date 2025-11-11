package router

import (
	"time"
	"github.com/gin-gonic/gin"
	"server_1/internal/core/config"
	"server_1/internal/modules/dynamicapi"
	"server_1/internal/modules/export"
	"server_1/internal/modules/leads"
	// "server_1/internal/modules/test_items"
	"server_1/internal/modules/reports"
	"server_1/internal/modules/schema"
)

func Build(cfg config.Config) *gin.Engine {
	r := gin.New()
	// Apply recovery, logging, and a conservative request timeout.
	// Keep this lower than server WriteTimeout to ensure graceful cancellations.
	r.Use(gin.Recovery(), RequestLogger(), WithTimeout(20*time.Second))

	base := r.Group(cfg.Server.BasePath)
	base.GET("/health", func(c *gin.Context) { c.String(200, "ok") })

	v1 := base.Group("/api/v1")
	// test_items.MountRoutes(v1)
	dynamicapi.MountRoutes(v1, cfg.APIKeys.Dynamic)
	export.MountRoutes(v1)
	leads.MountRoutes(v1)
	reports.MountRoutes(v1)
	schema.MountRoutes(v1)
	return r
}
