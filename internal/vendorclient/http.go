package vendorclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// HTTPVendorClient calls the VSS stub over HTTP.
type HTTPVendorClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPClient creates an HTTPVendorClient targeting the given base URL (e.g. "http://localhost:8081").
// If client is nil, a default http.Client is used.
func NewHTTPClient(baseURL string, client *http.Client) *HTTPVendorClient {
	if client == nil {
		client = &http.Client{}
	}
	return &HTTPVendorClient{
		baseURL:    baseURL,
		httpClient: client,
	}
}

// Validate sends a check-image validation request to the Vendor Service.
func (c *HTTPVendorClient) Validate(ctx context.Context, req ValidateRequest) (*ValidateResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("vendorclient: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/validate", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("vendorclient: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if req.Scenario != "" {
		httpReq.Header.Set("X-Scenario", req.Scenario)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("vendorclient: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vendorclient: unexpected status %d", resp.StatusCode)
	}

	var result ValidateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("vendorclient: decode response: %w", err)
	}

	return &result, nil
}
