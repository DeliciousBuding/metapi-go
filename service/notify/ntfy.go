package notify

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
)

// NtfyChannel sends notifications via a self-hosted ntfy server
// (https://ntfy.sh).
//
// Sends a POST to {NtfyUrl}/{NtfyTopic} with the message as the body and
// the title/priority in headers. If NtfyToken is set, sends it as a Bearer
// token. NtfyUrl may be the public https://ntfy.sh or a self-hosted instance.
type NtfyChannel struct{}

func (c *NtfyChannel) Name() string { return "ntfy" }

func (c *NtfyChannel) Send(cfg *config.Config, title, message, level, timeFootnote string) error {
	if !cfg.NtfyEnabled || cfg.NtfyUrl == "" || cfg.NtfyTopic == "" {
		return fmt.Errorf("ntfy not configured")
	}

	base := strings.TrimRight(cfg.NtfyUrl, "/")
	endpoint := base + "/" + strings.Trim(cfg.NtfyTopic, "/")
	body := fmt.Sprintf("%s\n\n%s\n\n%s", title, message, timeFootnote)

	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("ntfy request build failed: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Title", fmt.Sprintf("[metapi][%s] %s", strings.ToUpper(level), title))
	req.Header.Set("Priority", ntfyPriority(level))
	req.Header.Set("Tags", ntfyTags(level))
	if cfg.NtfyToken != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.NtfyToken)
	}

	// Use the shared, swap-able notify client (same as other channels) so
	// SSRF validation + bounded timeout + test client swap all apply.
	resp, err := doNotifyRequest(req)
	if err != nil {
		return fmt.Errorf("ntfy request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("ntfy response status %d", resp.StatusCode)
	}
	return nil
}

// ntfyPriority maps metapi levels to ntfy priority strings.
func ntfyPriority(level string) string {
	switch strings.ToLower(level) {
	case "error":
		return "urgent"
	case "warning":
		return "high"
	default:
		return "default"
	}
}

// ntfyTags returns emoji-ish tags per level for the ntfy notification.
func ntfyTags(level string) string {
	switch strings.ToLower(level) {
	case "error":
		return "rotating_light"
	case "warning":
		return "warning"
	default:
		return "information_source"
	}
}
