package export

import (
	"github.com/gin-gonic/gin"

	"server_1/internal/core/prism"
	"server_1/internal/modules/dynamicapi"
)

func MountRoutes(group *gin.RouterGroup, checker prism.Checker) {
	repo := dynamicapi.NewRepo()
	ctrl := NewController(repo)

	group.GET("/export", prism.RequirePrism(checker, "report:export"), ctrl.Export)
	group.POST("/export", prism.RequirePrism(checker, "report:export"), ctrl.Export)
}
