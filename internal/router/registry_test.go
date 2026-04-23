package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"server_1/internal/core/config"
)

func TestBuildLeavesHealthRouteUngarded(t *testing.T) {
	engine := Build(config.Config{
		Server: config.ServerConfig{
			BasePath: "",
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
}
