package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	apiKey   string
	endpoint string
	http     *http.Client
}

func NewClient(apiKey, endpoint string) *Client {
	return &Client{
		apiKey:   apiKey,
		endpoint: endpoint,
		http: &http.Client{
			Timeout: 300 * time.Second,
		},
	}
}

// doPost marshals req, POSTs to c.endpoint, and returns the open response body.
// The caller is responsible for closing the body. On non-200 the body is closed
// and an error is returned.
func (c *Client) doPost(ctx context.Context, req *Request) (*http.Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.setHeaders(httpReq)
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, c.parseError(resp)
	}
	return resp, nil
}

// StreamMessage sends a streaming request and dispatches events via the callback.
func (c *Client) StreamMessage(ctx context.Context, req *Request, cb StreamCallback) error {
	req.Stream = true
	resp, err := c.doPost(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return ParseSSEStream(resp.Body, cb)
}

// SendMessage sends a non-streaming request and returns the full response.
func (c *Client) SendMessage(ctx context.Context, req *Request) (*Response, error) {
	req.Stream = false
	resp, err := c.doPost(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result Response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// ListModels fetches all available models from the API, handling pagination.
// Derives the models endpoint from the configured messages endpoint.
func (c *Client) ListModels(ctx context.Context) ([]ModelInfo, error) {
	var allModels []ModelInfo
	// Derive models URL from the configured endpoint (e.g. ".../v1/messages" -> ".../v1/models")
	baseURL := strings.TrimSuffix(c.endpoint, "/messages") + "/models"
	afterID := ""

	for {
		reqURL := baseURL + "?limit=100"
		if afterID != "" {
			reqURL += "&after_id=" + afterID
		}

		httpReq, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		c.setHeaders(httpReq)

		resp, err := c.http.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("do request: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			err := c.parseError(resp)
			resp.Body.Close()
			return nil, err
		}

		var result ModelsResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode models response: %w", err)
		}
		resp.Body.Close()

		allModels = append(allModels, result.Data...)

		if !result.HasMore || result.LastID == "" {
			break
		}
		afterID = result.LastID
	}

	return allModels, nil
}

// ValidateModel checks if a model ID exists in the available models.
// Returns the model info if found, or nil with a list of suggestions if not.
func (c *Client) ValidateModel(ctx context.Context, modelID string) (*ModelInfo, []ModelInfo, error) {
	models, err := c.ListModels(ctx)
	if err != nil {
		return nil, nil, err
	}

	for i, m := range models {
		if m.ID == modelID {
			return &models[i], nil, nil
		}
	}

	// Model not found — find suggestions by matching prefix substrings
	var suggestions []ModelInfo
	// Extract base name (e.g., "claude-sonnet-4-6" from "claude-sonnet-4-6-20250620")
	parts := splitModelID(modelID)
	for _, m := range models {
		if matchesModelFamily(parts, m.ID) {
			suggestions = append(suggestions, m)
		}
	}

	return nil, suggestions, nil
}

// splitModelID splits a model ID into family parts for fuzzy matching.
func splitModelID(id string) string {
	// Try to strip the date suffix (last component after -)
	// e.g. "claude-sonnet-4-6-20250620" -> "claude-sonnet-4-6"
	parts := strings.Split(id, "-")
	if len(parts) > 1 {
		last := parts[len(parts)-1]
		if len(last) == 8 && last[0] >= '0' && last[0] <= '9' {
			return strings.Join(parts[:len(parts)-1], "-")
		}
	}
	return id
}

// matchesModelFamily checks if a model ID shares the same family prefix.
func matchesModelFamily(familyPrefix, candidateID string) bool {
	return strings.HasPrefix(candidateID, familyPrefix)
}

// Name implements Provider.
func (c *Client) Name() string { return "anthropic" }

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")
}

func (c *Client) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	var apiErr APIError
	apiErr.StatusCode = resp.StatusCode
	if err := json.Unmarshal(body, &apiErr); err != nil {
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
	return &apiErr
}

