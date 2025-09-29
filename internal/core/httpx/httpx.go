package httpx

import (
    "net/http"
    "github.com/gin-gonic/gin"
)

func OK(c *gin.Context, data interface{})   { c.JSON(http.StatusOK, data) }
func Fail(c *gin.Context, code int, data interface{}) { c.JSON(code, data) }
