package communications

import (
	"github.com/gin-gonic/gin"

	"server_1/internal/core/config"
	"server_1/internal/core/prism"
)

func MountRoutes(v1 *gin.RouterGroup, checker prism.Checker, cfg config.Config) {
	g := v1.Group("/communications/email", prism.RequirePrism(checker, "report:read"))
	ctl := &Controller{Service: NewEmailService(cfg.Email)}

	g.POST("/jobs", ctl.CreateJob)
	g.GET("/jobs/:jobId", ctl.GetJob)
	g.POST("/jobs/:jobId/retry", ctl.RetryJob)
	g.GET("/logs", ctl.ListLogs)
	g.GET("/references/:referenceType/:referenceId", ctl.GetReferenceJobs)
}
