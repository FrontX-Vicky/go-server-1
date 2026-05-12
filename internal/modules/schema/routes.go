package schema

import (
	"github.com/gin-gonic/gin"

	"server_1/internal/core/prism"
)

// MountRoutes registers schema endpoints under the provided group.
func MountRoutes(group *gin.RouterGroup, checker prism.Checker) {
	handler := NewHandler()
	group.GET("/schema", prism.RequirePrism(checker, "db-explorer:read"), handler.Get)
}
