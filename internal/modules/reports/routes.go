package reports

import (
	"github.com/gin-gonic/gin"

	"server_1/internal/core/prism"
)

func MountRoutes(v1 *gin.RouterGroup, checker prism.Checker) {
	g := v1.Group("/reports")
	ctl := &Controller{Repo: NewRepo()}
	//g.GET("", ctl.List)
	//g.POST("", ctl.Create)
	g.GET("/:id", prism.RequirePrismForReport(checker), ctl.Show)
	g.POST("/:id", prism.RequirePrismForReport(checker), ctl.Execute)
	//g.PUT("/:id", ctl.Update)
	//g.DELETE("/:id", ctl.Delete)
}
