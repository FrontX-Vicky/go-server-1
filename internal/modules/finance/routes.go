package finance

import (
	"github.com/gin-gonic/gin"

	"server_1/internal/core/prism"
)

func MountRoutes(v1 *gin.RouterGroup, checker prism.Checker) {
	g := v1.Group("/finance")
	ctl := &Controller{Repo: NewRepo()}
	g.POST("/franchisee-report", prism.RequirePrism(checker, "report:read"), ctl.FranchiseeReport)
}
