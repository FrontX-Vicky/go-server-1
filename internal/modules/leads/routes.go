package leads

import "github.com/gin-gonic/gin"

func MountRoutes(v1 *gin.RouterGroup) {
	g := v1.Group("/leads")
	ctl := &Controller{Repo: NewRepo()}
	g.POST("/top-summary", ctl.TopSummary)
	g.POST("/source-breakdown", ctl.SourceBreakdown)
	g.POST("/center-performance", ctl.CenterPerformance)
	g.POST("/funnel-tracking", ctl.FunnelStageTracking)
	g.POST("/query", ctl.Query)
}
