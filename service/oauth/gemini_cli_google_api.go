package oauth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

func fetchGcpProjects(accessToken string, proxyURL *string) ([]string, error) {
	req, err := http.NewRequest("GET", geminiCliProjectsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := doHTTP(req, proxyURL, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody := readOAuthErrorResponseBody(resp.Body)
		return nil, fmt.Errorf("%s", string(respBody))
	}

	respBody, err := readOAuthJSONResponseBody(resp.Body)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Projects []struct {
			ProjectID string `json:"projectId"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, err
	}

	var projects []string
	for _, p := range payload.Projects {
		if id := strings.TrimSpace(p.ProjectID); id != "" {
			projects = append(projects, id)
		}
	}
	return projects, nil
}

func fetchGeminiUserEmail(accessToken string, proxyURL *string) (string, error) {
	req, err := http.NewRequest("GET", geminiCliUserinfoURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := doHTTP(req, proxyURL, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", nil
	}

	var payload struct {
		Email string `json:"email"`
	}
	respBody, err := readOAuthJSONResponseBody(resp.Body)
	if err != nil {
		return "", err
	}
	_ = json.Unmarshal(respBody, &payload)
	return strings.TrimSpace(payload.Email), nil
}

func checkCloudAIAPIEnabled(accessToken, projectID string, proxyURL *string) error {
	checkURL := fmt.Sprintf("%s/projects/%s/services/%s",
		geminiCliServiceUsageURL, url.PathEscape(projectID), url.PathEscape(geminiCliRequiredService))

	req, err := http.NewRequest("GET", checkURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", geminiCliUserAgent)
	req.Header.Set("X-Goog-Api-Client", geminiCliGoogleAPIClient)

	resp, err := doHTTP(req, proxyURL, nil)
	if err == nil {
		defer resp.Body.Close()
		var payload struct {
			State string `json:"state"`
		}
		respBody, readErr := readOAuthJSONResponseBody(resp.Body)
		if readErr == nil {
			json.Unmarshal(respBody, &payload)
		}
		if strings.EqualFold(strings.TrimSpace(payload.State), "ENABLED") {
			return nil
		}
	}

	// Enable the service
	enableURL := fmt.Sprintf("%s/projects/%s/services/%s:enable",
		geminiCliServiceUsageURL, url.PathEscape(projectID), url.PathEscape(geminiCliRequiredService))

	enableReq, err := http.NewRequest("POST", enableURL, bytes.NewReader([]byte("{}")))
	if err != nil {
		return err
	}
	enableReq.Header.Set("Authorization", "Bearer "+accessToken)
	enableReq.Header.Set("Accept", "application/json")
	enableReq.Header.Set("Content-Type", "application/json")
	enableReq.Header.Set("User-Agent", geminiCliUserAgent)
	enableReq.Header.Set("X-Goog-Api-Client", geminiCliGoogleAPIClient)

	enableResp, err := doHTTP(enableReq, proxyURL, nil)
	if err != nil {
		return err
	}
	defer enableResp.Body.Close()

	enableBody, err := readOAuthJSONResponseBody(enableResp.Body)
	if err != nil {
		return err
	}
	if enableResp.StatusCode == 200 {
		return nil
	}
	if enableResp.StatusCode == 400 && strings.Contains(strings.ToLower(string(enableBody)), "already enabled") {
		return nil
	}
	return fmt.Errorf("project activation required: HTTP %d", enableResp.StatusCode)
}
