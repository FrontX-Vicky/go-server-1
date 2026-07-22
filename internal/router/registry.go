package router

import (
	"time"

	"github.com/gin-gonic/gin"
	ginprometheus "github.com/zsais/go-gin-prometheus"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"server_1/internal/core/config"
	"server_1/internal/core/prism"
	"server_1/internal/modules/communications"
	"server_1/internal/modules/dynamicapi"
	"server_1/internal/modules/expense"
	"server_1/internal/modules/export"
	"server_1/internal/modules/finance"
	"server_1/internal/modules/leads"
	"server_1/internal/modules/openapi"
	// "server_1/internal/modules/test_items"
	"server_1/internal/modules/reports"
	"server_1/internal/modules/schema"
)

func Build(cfg config.Config) *gin.Engine {
	r := gin.New()
	
	// Add Prometheus monitoring
	p := ginprometheus.NewPrometheus("gin")
	p.Use(r)
	
	// Apply OpenTelemetry middleware
	r.Use(otelgin.Middleware("markx-go-api"))
	
	// Apply recovery, logging, and a conservative request timeout.
	// Keep this lower than server WriteTimeout to ensure graceful cancellations.
	r.Use(gin.Recovery(), RequestLogger(), WithTimeout(20*time.Second))
	prismClient := prism.NewClient(cfg.Prism)

	base := r.Group(cfg.Server.BasePath)
	base.GET("/health", func(c *gin.Context) { c.String(200, "ok") })

	v1 := base.Group("/api/v1")
	// test_items.MountRoutes(v1)
	communications.MountRoutes(v1, prismClient, cfg)
	dynamicapi.MountRoutes(v1, cfg.APIKeys.Dynamic, prismClient)
	export.MountRoutes(v1, prismClient)
	expense.MountRoutes(v1, prismClient)
	finance.MountRoutes(v1, prismClient)
	leads.MountRoutes(v1, prismClient)
	openapi.MountRoutes(v1, cfg.APIKeys.Open)
	reports.MountRoutes(v1, prismClient)
	schema.MountRoutes(v1, prismClient)
	return r
}
