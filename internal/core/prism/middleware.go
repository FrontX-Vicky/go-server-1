package prism

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const parsedBodyContextKey = "prism.parsed_body"

func RequirePrism(checker Checker, action string) gin.HandlerFunc {
	return requirePrism(checker, nil, func(c *gin.Context) ([]CheckRequest, error) {
		return []CheckRequest{
			{
				Action:         action,
				ResourceType:   deriveResourceType(action),
				ResourceID:     "*",
				RequestContext: buildRequestContext(c),
			},
		}, nil
	})
}

func RequirePrismAny(checker Checker, actions ...string) gin.HandlerFunc {
	return requirePrism(checker, nil, func(c *gin.Context) ([]CheckRequest, error) {
		requests := make([]CheckRequest, 0, len(actions))
		for _, action := range actions {
			requests = append(requests, CheckRequest{
				Action:         action,
				ResourceType:   deriveResourceType(action),
				ResourceID:     "*",
				RequestContext: buildRequestContext(c),
			})
		}
		return requests, nil
	})
}

func RequirePrismWithApiKey(checker Checker, apiKey string, actions ...string) gin.HandlerFunc {
	return requirePrism(checker, func(c *gin.Context) bool {
		if apiKey == "" {
			return true
		}
		headerKey := c.GetHeader("X-API-Key")
		return subtleCompare(headerKey, apiKey)
	}, func(c *gin.Context) ([]CheckRequest, error) {
		requests := make([]CheckRequest, 0, len(actions))
		for _, action := range actions {
			requests = append(requests, CheckRequest{
				Action:         action,
				ResourceType:   deriveResourceType(action),
				ResourceID:     "*",
				RequestContext: buildRequestContext(c),
			})
		}
		return requests, nil
	})
}

func RequirePrismForReport(checker Checker) gin.HandlerFunc {
	return requirePrism(checker, nil, func(c *gin.Context) ([]CheckRequest, error) {
		reportID := strings.TrimSpace(c.Param("id"))
		action := "report:read"
		resourceType := "report"
		if reportID == "411" {
			action = "lead:list"
			resourceType = "lead"
		}

		ctx := buildRequestContext(c)
		ctx["report_id"] = reportID

		return []CheckRequest{
			{
				Action:         action,
				ResourceType:   resourceType,
				ResourceID:     reportID,
				RequestContext: ctx,
			},
		}, nil
	})
}

func requirePrism(
	checker Checker,
	apiKeyCheck func(*gin.Context) bool,
	resolve func(*gin.Context) ([]CheckRequest, error),
) gin.HandlerFunc {
	return func(c *gin.Context) {
		if apiKeyCheck != nil && !apiKeyCheck(c) {
			abortJSON(c, http.StatusUnauthorized, "missing or invalid API key")
			return
		}

		token, ok := extractBearerToken(c)
		if !ok {
			abortJSON(c, http.StatusUnauthorized, "Authorization: Bearer <token> header is required")
			return
		}

		requests, err := resolve(c)
		if err != nil {
			abortJSON(c, http.StatusForbidden, err.Error())
			return
		}

		var denyReason string
		for _, req := range requests {
			result, err := checker.Check(c.Request.Context(), token, req)
			if errors.Is(err, ErrUnauthorized) {
				abortJSON(c, http.StatusUnauthorized, "Invalid or expired access token")
				return
			}
			if err != nil {
				denyReason = "PRISM authorization service unavailable"
				continue
			}
			if result.Allowed {
				c.Next()
				return
			}
			if result.Reason != "" {
				denyReason = result.Reason
			}
		}

		if denyReason == "" {
			denyReason = "Access denied by PRISM policy"
		}
		abortJSON(c, http.StatusForbidden, denyReason)
	}
}

func extractBearerToken(c *gin.Context) (string, bool) {
	header := strings.TrimSpace(c.GetHeader("Authorization"))
	if header == "" || len(header) <= len("Bearer ") {
		return "", false
	}
	if !strings.EqualFold(header[:7], "Bearer ") {
		return "", false
	}

	token := strings.TrimSpace(header[7:])
	return token, token != ""
}

func buildRequestContext(c *gin.Context) map[string]any {
	ctx := map[string]any{
		"http_method":  c.Request.Method,
		"route_path":   c.FullPath(),
		"request_path": c.Request.URL.Path,
		"query_only":   resolveQueryOnly(c),
	}
	if table := bodyStringField(c, "table"); table != "" {
		ctx["table"] = table
	}
	return ctx
}

func resolveQueryOnly(c *gin.Context) bool {
	raw := strings.TrimSpace(strings.ToLower(c.Query("query_only")))
	if raw != "" {
		return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
	}

	body := parsedBody(c)
	if value, ok := body["query_only"]; ok {
		switch typed := value.(type) {
		case bool:
			return typed
		case string:
			normalized := strings.TrimSpace(strings.ToLower(typed))
			return normalized == "1" || normalized == "true" || normalized == "yes" || normalized == "on"
		}
	}

	return false
}

func bodyStringField(c *gin.Context, key string) string {
	body := parsedBody(c)
	value, ok := body[key]
	if !ok || value == nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func parsedBody(c *gin.Context) map[string]any {
	if cached, ok := c.Get(parsedBodyContextKey); ok {
		if body, ok := cached.(map[string]any); ok {
			return body
		}
	}

	empty := map[string]any{}
	if c.Request == nil || c.Request.Body == nil {
		c.Set(parsedBodyContextKey, empty)
		return empty
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil || len(bodyBytes) == 0 {
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		c.Set(parsedBodyContextKey, empty)
		return empty
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	parsed := map[string]any{}
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		parsed = empty
	}
	c.Set(parsedBodyContextKey, parsed)
	return parsed
}

func deriveResourceType(action string) string {
	parts := strings.SplitN(strings.TrimSpace(action), ":", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "*"
	}
	return parts[0]
}

func abortJSON(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{
			"message": message,
		},
	})
}

func subtleCompare(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	var diff byte
	for i := 0; i < len(left); i++ {
		diff |= left[i] ^ right[i]
	}
	return diff == 0
}
