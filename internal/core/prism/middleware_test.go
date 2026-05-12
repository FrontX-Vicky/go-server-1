package prism

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type stubChecker struct {
	check func(ctx context.Context, bearerToken string, req CheckRequest) (CheckResult, error)
}

func (s stubChecker) Check(ctx context.Context, bearerToken string, req CheckRequest) (CheckResult, error) {
	return s.check(ctx, bearerToken, req)
}

func TestRequirePrismMissingAuthorizationReturns401(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.GET("/protected", RequirePrism(stubChecker{
		check: func(ctx context.Context, bearerToken string, req CheckRequest) (CheckResult, error) {
			t.Fatal("checker should not be called when auth is missing")
			return CheckResult{}, nil
		},
	}, "report:read"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
}

func TestRequirePrismDeniedReturns403(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.GET("/protected", RequirePrism(stubChecker{
		check: func(ctx context.Context, bearerToken string, req CheckRequest) (CheckResult, error) {
			return CheckResult{Allowed: false, Reason: "denied"}, nil
		},
	}, "report:read"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", recorder.Code)
	}
}

func TestRequirePrismAllowedReachesHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.GET("/protected", RequirePrism(stubChecker{
		check: func(ctx context.Context, bearerToken string, req CheckRequest) (CheckResult, error) {
			return CheckResult{Allowed: true}, nil
		},
	}, "report:read"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", recorder.Code)
	}
}

func TestRequirePrismWithApiKeyRejectsMissingAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.POST("/dynamic/fetch", RequirePrismWithApiKey(stubChecker{
		check: func(ctx context.Context, bearerToken string, req CheckRequest) (CheckResult, error) {
			t.Fatal("checker should not be called when api key is missing")
			return CheckResult{}, nil
		},
	}, "dynamic-secret", "report:read"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/dynamic/fetch", nil)
	req.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
}

func TestRequirePrismWithApiKeyRejectsDeniedPrism(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.POST("/dynamic/fetch", RequirePrismWithApiKey(stubChecker{
		check: func(ctx context.Context, bearerToken string, req CheckRequest) (CheckResult, error) {
			return CheckResult{Allowed: false, Reason: "denied"}, nil
		},
	}, "dynamic-secret", "report:read"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/dynamic/fetch", nil)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("X-API-Key", "dynamic-secret")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", recorder.Code)
	}
}

func TestRequirePrismForReportMapsInquiryBoardToLeadList(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var capturedAction string
	engine := gin.New()
	engine.GET("/reports/:id", RequirePrismForReport(stubChecker{
		check: func(ctx context.Context, bearerToken string, req CheckRequest) (CheckResult, error) {
			capturedAction = req.Action
			return CheckResult{Allowed: true}, nil
		},
	}), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/reports/411", nil)
	req.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", recorder.Code)
	}
	if capturedAction != "lead:list" {
		t.Fatalf("expected lead:list, got %q", capturedAction)
	}
}

func TestRequirePrismForReportMapsOtherReportsToReportRead(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var capturedAction string
	engine := gin.New()
	engine.GET("/reports/:id", RequirePrismForReport(stubChecker{
		check: func(ctx context.Context, bearerToken string, req CheckRequest) (CheckResult, error) {
			capturedAction = req.Action
			return CheckResult{Allowed: true}, nil
		},
	}), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/reports/42", nil)
	req.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", recorder.Code)
	}
	if capturedAction != "report:read" {
		t.Fatalf("expected report:read, got %q", capturedAction)
	}
}

func TestRequirePrismServiceFailureDeniesClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.GET("/protected", RequirePrism(stubChecker{
		check: func(ctx context.Context, bearerToken string, req CheckRequest) (CheckResult, error) {
			return CheckResult{}, errors.New("timeout")
		},
	}, "report:read"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer token")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", recorder.Code)
	}
}
