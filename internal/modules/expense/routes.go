package expense

import (
	"github.com/gin-gonic/gin"

	"server_1/internal/core/prism"
)

// MountRoutes registers the expense endpoints under /api/v1/expense.
func MountRoutes(v1 *gin.RouterGroup, checker prism.Checker) {
	g := v1.Group("/expense", prism.RequirePrism(checker, "expense:read"))
	ctl := &Controller{Repo: NewRepo()}

	g.GET("", ctl.List)
	g.POST("", ctl.Execute)
}
