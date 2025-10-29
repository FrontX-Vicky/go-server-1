package export

import (
	"github.com/gin-gonic/gin"

	"server_1/internal/modules/dynamicapi"
)

func MountRoutes(group *gin.RouterGroup) {
	repo := dynamicapi.NewRepo()
	ctrl := NewController(repo)

	group.GET("/export", ctrl.Export)
	group.POST("/export", ctrl.Export)
}
