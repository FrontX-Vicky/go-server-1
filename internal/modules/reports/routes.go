package reports

import "github.com/gin-gonic/gin"

func MountRoutes(v1 *gin.RouterGroup) {
    g := v1.Group("/reports")
    ctl := &Controller{Repo: NewRepo()}
    g.GET("", ctl.List)
    g.POST("", ctl.Create)
    g.GET("/:id", ctl.Show)
    g.PUT("/:id", ctl.Update)
    g.DELETE("/:id", ctl.Delete)
}
