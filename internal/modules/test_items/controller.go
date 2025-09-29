package test_items

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"server_1/internal/core/httpx"
)

type Controller struct{ Repo *Repo }

type Item struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Status    int    `json:"status"`
	CreatedAt string `json:"created_at"`
}

// GET /api/v1/test_items
func (ctl *Controller) List(c *gin.Context) {
	rows, err := ctl.Repo.List(c.Request.Context(), 100, 0)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	httpx.OK(c, gin.H{"data": rows})
}

// GET /api/v1/test_items/:id
func (ctl *Controller) Show(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	row, err := ctl.Repo.Get(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Fail(c, http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		// Surfacing raw DB errors (e.g., table doesn't exist)
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	httpx.OK(c, row)
}

// POST /api/v1/test_items
func (ctl *Controller) Create(c *gin.Context) {
	var in struct {
		Name   string `json:"name"`
		Status int    `json:"status"`
	}
	if err := c.BindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	id, err := ctl.Repo.Create(c.Request.Context(), in.Name, in.Status)
	if err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	httpx.OK(c, gin.H{"id": id})
}

// PUT /api/v1/test_items/:id
func (ctl *Controller) Update(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var in struct {
		Name   *string `json:"name"`
		Status *int    `json:"status"`
	}
	if err := c.BindJSON(&in); err != nil {
		httpx.Fail(c, http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		in.Name = &n
	}
	if err := ctl.Repo.Update(c.Request.Context(), id, in.Name, in.Status); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}

// DELETE /api/v1/test_items/:id
func (ctl *Controller) Delete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := ctl.Repo.Delete(c.Request.Context(), id); err != nil {
		httpx.Fail(c, http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	httpx.OK(c, gin.H{"ok": true})
}