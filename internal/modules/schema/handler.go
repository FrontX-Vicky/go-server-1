package schema

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"server_1/internal/core/httpx"
)

const (
	defaultTTL        = 5 * time.Minute
	requestTimeout    = 15 * time.Second
	refreshQueryParam = "refresh"
)

// Handler exposes HTTP endpoints for the schema module.
type Handler struct {
	collector *Collector
	cache     *cache
}

// NewHandler creates a schema handler with default dependencies.
func NewHandler() *Handler {
	return &Handler{
		collector: NewCollector(),
		cache:     newCache(defaultTTL),
	}
}

// Get handles GET /api/schema.
func (h *Handler) Get(c *gin.Context) {
	bypass := shouldBypassCache(c.Query(refreshQueryParam))
	if bypass {
		h.cache.invalidate()
	} else if payload, ok := h.cache.get(); ok && payload != nil {
		httpx.OK(c, payload)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	tables, relations, truncTables, truncRelations, err := h.collector.Collect(ctx)
	if err != nil {
		status := http.StatusInternalServerError
		message := gin.H{"error": err.Error()}
		if errors.Is(err, ErrDBUnavailable) {
			status = http.StatusServiceUnavailable
			message = gin.H{"error": "database unavailable"}
		}
		httpx.Fail(c, status, message)
		return
	}

	now := time.Now().UTC()
	payload := &Payload{
		Tables:    tables,
		Relations: relations,
		Metrics: Metrics{
			LastRefreshed: now.Format(time.RFC3339),
			Truncated:     truncTables || truncRelations,
		},
	}

	h.cache.set(payload)
	httpx.OK(c, payload)
}

func shouldBypassCache(refresh string) bool {
	if refresh == "" {
		return false
	}
	refresh = strings.TrimSpace(strings.ToLower(refresh))
	return refresh == "1" || refresh == "true" || refresh == "yes"
}
