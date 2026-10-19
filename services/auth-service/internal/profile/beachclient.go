package profile

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// BeachChecker verifies that a home beach exists in beach-service.
type BeachChecker interface {
	Exists(ctx context.Context, beachID string) (bool, error)
}

// HTTPBeachChecker calls beach-service directly over the internal network.
type HTTPBeachChecker struct {
	baseURL string
	client  *http.Client
}

func NewHTTPBeachChecker(baseURL string) *HTTPBeachChecker {
	return &HTTPBeachChecker{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 3 * time.Second},
	}
}

func (c *HTTPBeachChecker) Exists(ctx context.Context, beachID string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/v1/beaches/"+url.PathEscape(beachID), nil)
	if err != nil {
		return false, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("call beach-service: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("beach-service returned %d", resp.StatusCode)
	}
}
