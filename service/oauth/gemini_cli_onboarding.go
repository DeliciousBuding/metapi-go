package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ---- Onboarding ----

type geminiLoadCodeAssistPayload struct {
	CloudaicompanionProject interface{} `json:"cloudaicompanionProject"`
	AllowedTiers            interface{} `json:"allowedTiers"`
}

type geminiInternalMetadata struct {
	IDEType    string `json:"ideType"`
	Platform   string `json:"platform"`
	PluginType string `json:"pluginType"`
}

func buildGeminiCliMetadata() geminiInternalMetadata {
	return geminiInternalMetadata{
		IDEType:    "IDE_UNSPECIFIED",
		Platform:   "PLATFORM_UNSPECIFIED",
		PluginType: "GEMINI",
	}
}

func extractGeminiProjectID(value interface{}) string {
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	if m, ok := value.(map[string]interface{}); ok {
		if id, ok := m["id"]; ok {
			if s, ok := id.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func extractGeminiDefaultTierID(payload *geminiLoadCodeAssistPayload) string {
	tiers, ok := payload.AllowedTiers.([]interface{})
	if !ok {
		return "legacy-tier"
	}
	for _, rawTier := range tiers {
		tier, ok := rawTier.(map[string]interface{})
		if !ok {
			continue
		}
		if isDefault, _ := tier["isDefault"].(bool); isDefault {
			if id, ok := tier["id"].(string); ok {
				trimmed := strings.TrimSpace(id)
				if trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return "legacy-tier"
}

func isGeminiFreeUserProject(requestedProjectID, tierID string) bool {
	return strings.HasPrefix(strings.TrimSpace(requestedProjectID), "gen-lang-client-") ||
		strings.EqualFold(strings.TrimSpace(tierID), "FREE") ||
		strings.EqualFold(strings.TrimSpace(tierID), "LEGACY")
}

func isSameGeminiProjectID(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func callGeminiCliInternalAPI(ctx context.Context, accessToken, method string, body map[string]interface{}, proxyURL *string) (map[string]interface{}, error) {
	bodyBytes, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/%s:%s", geminiCliUpstreamBaseURL, geminiCliInternalAPIVersion, method),
		bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", geminiCliUserAgent)
	req.Header.Set("X-Goog-Api-Client", geminiCliGoogleAPIClient)

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

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func performGeminiCliSetup(ctx context.Context, accessToken, requestedProjectID string, proxyURL *string) (string, error) {
	trimmedRequest := strings.TrimSpace(requestedProjectID)
	explicitProject := trimmedRequest != ""
	metadata := buildGeminiCliMetadata()

	loadBody := map[string]interface{}{"metadata": metadata}
	if explicitProject {
		loadBody["cloudaicompanionProject"] = trimmedRequest
	}

	loadResp, err := callGeminiCliInternalAPI(ctx, accessToken, "loadCodeAssist", loadBody, proxyURL)
	if err != nil {
		return "", err
	}

	loadPayload := &geminiLoadCodeAssistPayload{}
	if v, ok := loadResp["cloudaicompanionProject"]; ok {
		loadPayload.CloudaicompanionProject = v
	}
	if v, ok := loadResp["allowedTiers"]; ok {
		loadPayload.AllowedTiers = v
	}

	tierID := extractGeminiDefaultTierID(loadPayload)
	projectID := trimmedRequest
	if projectID == "" {
		projectID = extractGeminiProjectID(loadPayload.CloudaicompanionProject)
	}

	if projectID == "" {
		for attempt := 0; attempt < geminiCliAutoOnboardMaxAttempts; attempt++ {
			onboardResp, err := callGeminiCliInternalAPI(ctx, accessToken, "onboardUser", map[string]interface{}{
				"tierId":   tierID,
				"metadata": metadata,
			}, proxyURL)
			if err != nil {
				return "", err
			}
			if done, _ := onboardResp["done"].(bool); done {
				if resp, ok := onboardResp["response"].(map[string]interface{}); ok {
					projectID = extractGeminiProjectID(resp["cloudaicompanionProject"])
				}
				break
			}
			if attempt+1 < geminiCliAutoOnboardMaxAttempts {
				if err := sleepCtx(ctx, time.Duration(geminiCliAutoOnboardPollIntervalMs)*time.Millisecond); err != nil {
					return "", err
				}
			}
		}
	}

	if projectID == "" {
		return "", fmt.Errorf("gemini cli: project selection required")
	}

	finalProjectID := projectID
	for attempt := 0; attempt < geminiCliOnboardMaxAttempts; attempt++ {
		onboardResp, err := callGeminiCliInternalAPI(ctx, accessToken, "onboardUser", map[string]interface{}{
			"tierId":                  tierID,
			"metadata":                metadata,
			"cloudaicompanionProject": projectID,
		}, proxyURL)
		if err != nil {
			return "", err
		}
		if done, _ := onboardResp["done"].(bool); done {
			responseProjectID := ""
			if resp, ok := onboardResp["response"].(map[string]interface{}); ok {
				responseProjectID = extractGeminiProjectID(resp["cloudaicompanionProject"])
			}
			if responseProjectID != "" {
				if explicitProject && !isSameGeminiProjectID(responseProjectID, projectID) {
					if isGeminiFreeUserProject(projectID, tierID) {
						finalProjectID = responseProjectID
					} else {
						finalProjectID = projectID
					}
				} else {
					finalProjectID = responseProjectID
				}
			}
			if finalProjectID == "" {
				finalProjectID = projectID
			}
			return finalProjectID, nil
		}
		if attempt+1 < geminiCliOnboardMaxAttempts {
			if err := sleepCtx(ctx, time.Duration(geminiCliOnboardPollIntervalMs)*time.Millisecond); err != nil {
				return "", err
			}
		}
	}

	if finalProjectID != "" {
		return finalProjectID, nil
	}
	return "", fmt.Errorf("gemini cli: onboarding timed out")
}

func ensureGeminiProjectAndOnboard(ctx context.Context, accessToken, requestedProjectID string, proxyURL *string) (string, error) {
	explicitProject := strings.TrimSpace(requestedProjectID)
	if explicitProject != "" {
		return performGeminiCliSetup(ctx, accessToken, explicitProject, proxyURL)
	}

	projects, err := fetchGcpProjects(accessToken, proxyURL)
	if err != nil {
		return "", err
	}
	if len(projects) == 0 {
		return "", fmt.Errorf("no Google Cloud projects available for this account")
	}
	return performGeminiCliSetup(ctx, accessToken, projects[0], proxyURL)
}
