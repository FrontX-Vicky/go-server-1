package schema

import "github.com/gin-gonic/gin"

// MountRoutes registers schema endpoints under the provided group.
func MountRoutes(group *gin.RouterGroup) {
	handler := NewHandler()
	group.GET("/schema", handler.Get)
}
