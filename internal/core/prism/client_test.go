package prism

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"server_1/internal/core/config"
)

func TestClientCheckForwardsHeadersAndPayload(t *testing.T) {
	var authHeader string
	var apiKeyHeader string
	var payload CheckRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		apiKeyHeader = r.Header.Get("X-API-Key")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"allowed":true,"decision":"Allow","reason":"ok"}`))
	}))
	defer server.Close()

	client := NewClient(config.PrismConfig{
		BaseURL:   server.URL,
		TimeoutMS: 2000,
		APIKey:    "service-secret",
	})

	result, err := client.Check(context.Background(), "access-token", CheckRequest{
		Action:       "report:read",
		ResourceType: "report",
		ResourceID:   "*",
		RequestContext: map[string]any{
			"route_path": "/api/v1/reports/42",
		},
	})
	if err != nil {
		t.Fatalf("check returned error: %v", err)
	}
	if !result.Allowed {
		t.Fatalf("expected allowed result, got %#v", result)
	}
	if authHeader != "Bearer access-token" {
		t.Fatalf("expected auth header to be forwarded, got %q", authHeader)
	}
	if apiKeyHeader != "service-secret" {
		t.Fatalf("expected api key header to be forwarded, got %q", apiKeyHeader)
	}
	if payload.Action != "report:read" {
		t.Fatalf("expected action to be forwarded, got %q", payload.Action)
	}
	if payload.RequestContext["route_path"] != "/api/v1/reports/42" {
		t.Fatalf("expected request context to include route_path, got %#v", payload.RequestContext)
	}
}

func TestClientCheckTimeoutReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"allowed":true}`))
	}))
	defer server.Close()

	client := NewClient(config.PrismConfig{
		BaseURL:   server.URL,
		TimeoutMS: 5,
	})

	_, err := client.Check(context.Background(), "access-token", CheckRequest{
		Action:       "report:read",
		ResourceType: "report",
		ResourceID:   "*",
	})
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}
}
