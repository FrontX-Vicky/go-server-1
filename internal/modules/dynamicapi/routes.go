package dynamicapi

import "github.com/gin-gonic/gin"

func MountRoutes(group *gin.RouterGroup, apiKey string) {
	repo := NewRepo()
	ctrl := NewController(repo, apiKey)

	group.POST("/dynamic/fetch", ctrl.Fetch)
}
