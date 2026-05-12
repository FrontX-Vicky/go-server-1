package leads

import (
	"github.com/gin-gonic/gin"

	"server_1/internal/core/prism"
)

func MountRoutes(v1 *gin.RouterGroup, checker prism.Checker) {
	g := v1.Group("/leads")
	ctl := &Controller{Repo: NewRepo()}
	g.POST("/top-summary", prism.RequirePrism(checker, "top-summary:read"), ctl.TopSummary)
	g.POST("/source-breakdown", prism.RequirePrism(checker, "report:read"), ctl.SourceBreakdown)
	g.POST("/center-performance", prism.RequirePrism(checker, "report:read"), ctl.CenterPerformance)
	g.POST("/funnel-tracking", prism.RequirePrism(checker, "report:read"), ctl.FunnelStageTracking)
	g.POST("/campaign-performance", prism.RequirePrism(checker, "report:read"), ctl.CampaignPerformance)
	g.POST("/heard-from-performance", prism.RequirePrism(checker, "report:read"), ctl.HeardFromPerformance)
	g.POST("/query", prism.RequirePrism(checker, "lead:list"), ctl.Query)
}
