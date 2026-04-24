package openapi

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func RequireAPIKey(apiKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		expected := strings.TrimSpace(apiKey)
		if expected == "" {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"error": gin.H{"message": "OPEN_API_KEY is not configured"},
			})
			return
		}

		provided := strings.TrimSpace(c.GetHeader("X-API-Key"))
		if provided == "" {
			provided = bearerToken(c.GetHeader("Authorization"))
		}

		if !sameSecret(provided, expected) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{"message": "missing or invalid API key"},
			})
			return
		}

		c.Next()
	}
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if len(header) <= len("Bearer ") || !strings.EqualFold(header[:7], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(header[7:])
}

func sameSecret(left, right string) bool {
	if left == "" || right == "" || len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}
