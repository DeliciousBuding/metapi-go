package notify

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service"
)

// TelegramChannel sends notifications via Telegram Bot.
type TelegramChannel struct{}

func (c *TelegramChannel) Name() string { return "telegram" }

// telegramHTTPClient is the default (direct) client. Tests replace it to
// exercise response handling without a network; production uses the
// notify transport's public-only dial guard.
var telegramHTTPClient = &http.Client{
	Timeout:       30 * time.Second,
	Transport:     newNotifyHTTPTransport(),
	CheckRedirect: platform.RejectCrossOriginRedirect,
}

func (c *TelegramChannel) Send(cfg *config.RuntimeSettings, title, message, level, timeFootnote string) error {
	if !cfg.TelegramEnabled || cfg.TelegramBotToken == "" || cfg.TelegramChatId == "" {
		return fmt.Errorf("telegram not configured")
	}

	apiBase := strings.TrimRight(cfg.TelegramApiBaseUrl, "/")
	if apiBase == "" {
		apiBase = "https://api.telegram.org"
	}

	reqURL := fmt.Sprintf("%s/bot%s/sendMessage", apiBase, cfg.TelegramBotToken)
	parsedURL, err := url.Parse(reqURL)
	if err != nil {
		return fmt.Errorf("telegram API base URL is invalid")
	}
	// The API base URL is admin-controlled, so it is an outbound target the
	// session could repoint. Reject loopback/RFC1918/ULA/metadata before any
	// dial; the transport additionally pins the checked DNS answer and
	// rejects cross-origin redirects (audit M2).
	if err := validateNotifyRequestURL(parsedURL); err != nil {
		return fmt.Errorf("telegram request target rejected: %w", err)
	}

	text := buildTelegramText(title, message, level, timeFootnote)

	bodyMap := map[string]any{
		"chat_id":                  cfg.TelegramChatId,
		"text":                     text,
		"disable_web_page_preview": true,
	}

	threadIDStr := strings.TrimSpace(cfg.TelegramMessageThreadId)
	if threadIDStr != "" {
		if threadID, err := strconv.ParseInt(threadIDStr, 10, 64); err == nil && threadID > 0 {
			bodyMap["message_thread_id"] = threadID
		}
	}

	body, _ := json.Marshal(bodyMap)

	// Use the configured system proxy if requested; otherwise the guarded
	// direct client. Both reject cross-origin redirects.
	client := telegramHTTPClient
	if cfg.TelegramUseSystemProxy && cfg.SystemProxyUrl != "" {
		client = service.ProxyAwareHTTPClient(cfg.SystemProxyUrl, 30*time.Second)
	}

	req, err := http.NewRequest(http.MethodPost, reqURL, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("telegram request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("telegram response status %d", resp.StatusCode)
	}

	respBody, err := readNotifyResponseBody(resp.Body)
	if err != nil {
		return fmt.Errorf("telegram response read failed: %w", err)
	}
	var payload struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(respBody, &payload); err == nil && !payload.OK {
		// Never echo the upstream body: a repointed API base could reflect
		// arbitrary internal response content into the admin-facing error
		// (audit M2).
		return fmt.Errorf("telegram API rejected the message")
	}

	return nil
}

func buildTelegramText(title, message, level, timeFootnote string) string {
	const maxLen = 3900
	raw := fmt.Sprintf("[metapi][%s] %s\n\n%s\n\nLevel: %s\n%s",
		strings.ToUpper(level), title, message, level, timeFootnote)
	if len(raw) <= maxLen {
		return raw
	}
	return raw[:maxLen] + "\n\n...(truncated)"
}
