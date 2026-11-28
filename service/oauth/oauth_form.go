package oauth

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// postOAuthForm POSTs an x-www-form-urlencoded body and returns the raw JSON
// payload. Non-2xx responses are returned as errors with the trimmed body so
// callers can surface the provider error description.
func postOAuthForm(ctx context.Context, endpoint string, form url.Values, proxyURL *string) ([]byte, error) {
	refresh := form.Get("grant_type") == "refresh_token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, refreshPreparationFailure(refresh, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := doRefreshHTTP(req, proxyURL, refresh)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := readOAuthErrorResponseBody(resp.Body)
		return nil, refreshHTTPFailure(ctx, refresh, resp.StatusCode, body, fmt.Errorf("xAI returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
	}

	body, err := readOAuthJSONResponseBody(resp.Body)
	if err != nil {
		return nil, successfulRefreshResponseError(refresh, "response read failed", err)
	}
	return body, nil
}
