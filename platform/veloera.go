package platform

import (
	"context"
	"fmt"
	"strings"
)

// VeloeraAdapter handles Veloera platforms (1M divisor + Veloera-User header).
// Directly extends BaseAdapter, NOT NewApiAdapter.
type VeloeraAdapter struct {
	*BaseAdapter
}

// Detect probes GET /api/status and checks for "veloera" in system_name or version.
func (v *VeloeraAdapter) Detect(ctx context.Context, url string) (bool, error) {
	resp, err := fetchJSON(ctx, url+"/api/status", "GET", nil, nil, nil)
	if err != nil {
		return false, nil
	}

	success, _ := getBool(resp, "success")
	if !success {
		return false, nil
	}

	data, ok := getMap(resp, "data")
	if !ok {
		return false, nil
	}

	systemName, _ := getString(data, "system_name")
	systemName = strings.ToLower(systemName)
	version, _ := getString(data, "version")
	version = strings.ToLower(version)

	return strings.Contains(systemName, "veloera") || strings.Contains(version, "veloera"), nil
}

// veloeraHeaders sets Authorization + Veloera-User + New-API-User + User-id.
func veloeraHeaders(accessToken string, userID *int) map[string]string {
	headers := map[string]string{
		"Authorization": "Bearer " + accessToken,
	}
	if userID != nil {
		val := fmt.Sprintf("%d", *userID)
		headers["Veloera-User"] = val
		headers["New-API-User"] = val
		headers["User-id"] = val
	}
	return headers
}

// Checkin: POST /api/user/checkin (veloeraHeaders, requires platformUserId).
func (v *VeloeraAdapter) Checkin(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) (*CheckinResult, error) {
	headers := veloeraHeaders(accessToken, platformUserId)
	resp, err := fetchJSON(ctx, baseURL+"/api/user/checkin", "POST", nil, headers, proxy)
	if err != nil {
		return &CheckinResult{Success: false, Message: err.Error()}, nil
	}

	return checkinResultFromResponse(resp, "Check-in successful", "Check-in failed"), nil
}

// GetBalance: quota=total, balance=quota-used, divisor=1,000,000 (NOT 500,000!).
func (v *VeloeraAdapter) GetBalance(ctx context.Context, baseURL, accessToken string, platformUserId *int, proxy *ProxyConfig) (*BalanceInfo, error) {
	headers := veloeraHeaders(accessToken, platformUserId)
	resp, err := fetchJSON(ctx, baseURL+"/api/user/self", "GET", nil, headers, proxy)
	if err != nil {
		return &BalanceInfo{}, nil
	}

	balance := parseOneApiStyleBalance(resp, 1000000, false)
	return &balance, nil
}

// GetModels: GET /v1/models (Bearer auth only, no Veloera headers).
func (v *VeloeraAdapter) GetModels(ctx context.Context, baseURL string, apiToken string, platformUserId *int, proxy *ProxyConfig) ([]string, error) {
	headers := authBearerHeaders(apiToken)
	resp, err := fetchJSON(ctx, baseURL+"/v1/models", "GET", nil, headers, proxy)
	if err != nil {
		return nil, err
	}

	return extractModelIDsFromData(resp), nil
}
