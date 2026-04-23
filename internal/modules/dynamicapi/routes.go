package dynamicapi

import (
	"github.com/gin-gonic/gin"

	"server_1/internal/core/prism"
)

func MountRoutes(group *gin.RouterGroup, apiKey string, checker prism.Checker) {
	repo := NewRepo()
	ctrl := NewController(repo)

	group.POST("/dynamic/fetch", prism.RequirePrismWithApiKey(checker, apiKey, "report:read"), ctrl.Fetch)
}
