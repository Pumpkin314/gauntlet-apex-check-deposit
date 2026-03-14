package cloudauth

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Transport is an http.RoundTripper that injects a Cloud Run identity token
// via the X-Serverless-Authorization header for service-to-service auth.
// The existing Authorization header (app-level tokens) passes through untouched.
type Transport struct {
	Base     http.RoundTripper
	Audience string

	mu     sync.Mutex
	token  string
	expiry time.Time
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	token, err := t.getToken()
	if err != nil || token == "" {
		// Not on Cloud Run — proceed without identity token
		return base.RoundTrip(req)
	}

	clone := req.Clone(req.Context())
	clone.Header.Set("X-Serverless-Authorization", "Bearer "+token)
	return base.RoundTrip(clone)
}

func (t *Transport) getToken() (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.token != "" && time.Now().Before(t.expiry) {
		return t.token, nil
	}

	url := fmt.Sprintf(
		"http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity?audience=%s",
		t.Audience,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata server returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	t.token = string(body)
	t.expiry = time.Now().Add(50 * time.Minute)
	return t.token, nil
}

// NewClient creates an *http.Client that automatically injects Cloud Run
// identity tokens for service-to-service auth.
func NewClient(audience string) *http.Client {
	return &http.Client{
		Transport: &Transport{Audience: audience},
	}
}
