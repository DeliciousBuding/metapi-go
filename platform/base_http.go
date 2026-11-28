package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	platformJSONResponseBodyLimit  = 4 << 20
	platformTextResponseBodyLimit  = 2 << 20
	platformErrorResponseBodyLimit = 64 << 10
)

// fetchJSON performs an HTTP request and parses the JSON response body into a map.
func fetchJSON(ctx context.Context, url, method string, body interface{}, headers map[string]string, proxy *ProxyConfig) (map[string]interface{}, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = strings.NewReader(string(b))
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if headers == nil {
		headers = make(map[string]string)
	}
	if _, ok := headers["Content-Type"]; !ok {
		headers["Content-Type"] = "application/json"
	}
	if _, ok := headers["User-Agent"]; !ok {
		headers["User-Agent"] = DefaultBrowserUserAgent
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	ApplySiteIdentity(req, proxy)

	resp, err := DoWithProxy(ctx, req, proxy)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	bodyLimit := int64(platformJSONResponseBodyLimit)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyLimit = platformErrorResponseBodyLimit
	}
	respBody, err := readPlatformResponseBody(resp.Body, bodyLimit)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := extractResponseMessageFromBytes(respBody)
		if msg == "" {
			msg = string(respBody)
			if len(msg) > 200 {
				msg = msg[:200]
			}
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}
	return result, nil
}

func fetchText(ctx context.Context, url string, proxy *ProxyConfig) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")
	req.Header.Set("User-Agent", DefaultBrowserUserAgent)
	ApplySiteIdentity(req, proxy)

	resp, err := DoWithProxy(ctx, req, proxy)
	if err != nil {
		return "", "", fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	body, err := readPlatformResponseBody(resp.Body, platformTextResponseBodyLimit)
	if err != nil {
		return "", "", fmt.Errorf("read body: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")
	return string(body), contentType, nil
}

func readPlatformResponseBody(body io.Reader, limit int64) ([]byte, error) {
	limited := io.LimitReader(body, limit+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response body exceeds %d bytes", limit)
	}
	return data, nil
}
