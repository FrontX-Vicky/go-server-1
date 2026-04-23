package prism

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"server_1/internal/core/config"
)

var ErrUnauthorized = errors.New("prism unauthorized")

type CheckRequest struct {
	Action         string         `json:"action"`
	ResourceType   string         `json:"resource_type"`
	ResourceID     string         `json:"resource_id"`
	RequestContext map[string]any `json:"request_context,omitempty"`
}

type CheckResult struct {
	Allowed      bool   `json:"allowed"`
	Decision     string `json:"decision"`
	Reason       string `json:"reason"`
	Action       string `json:"action"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
}

type Checker interface {
	Check(ctx context.Context, bearerToken string, req CheckRequest) (CheckResult, error)
}

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

func NewClient(cfg config.PrismConfig) *Client {
	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		apiKey:  strings.TrimSpace(cfg.APIKey),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) Check(ctx context.Context, bearerToken string, req CheckRequest) (CheckResult, error) {
	if c == nil || c.baseURL == "" {
		return CheckResult{}, errors.New("prism api base url is not configured")
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return CheckResult{}, fmt.Errorf("marshal prism request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/prism/evaluate/me/check",
		bytes.NewReader(payload),
	)
	if err != nil {
		return CheckResult{}, fmt.Errorf("build prism request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(bearerToken))
	if c.apiKey != "" {
		httpReq.Header.Set("X-API-Key", c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return CheckResult{}, fmt.Errorf("perform prism request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return CheckResult{}, fmt.Errorf("read prism response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return CheckResult{}, ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CheckResult{}, fmt.Errorf("prism request failed with status %d", resp.StatusCode)
	}

	var result CheckResult
	if err := json.Unmarshal(body, &result); err != nil {
		return CheckResult{}, fmt.Errorf("decode prism response: %w", err)
	}

	return result, nil
}
