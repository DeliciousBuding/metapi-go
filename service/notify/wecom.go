package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/deliciousbuding/metapi-go/config"
)

// WecomChannel sends notifications via a WeCom (企业微信) custom-bot webhook
// (https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=...).
//
// Dedicated channel so the operator gets a separate enable toggle + config
// field from the generic webhook. Reuses buildWeComText from webhook.go for
// the content body (max-length aware) so rendering stays consistent.
type WecomChannel struct{}

func (c *WecomChannel) Name() string { return "wecom" }

func (c *WecomChannel) Send(cfg *config.Config, title, message, level, timeFootnote string) error {
	if !cfg.WecomEnabled || cfg.WecomWebhook == "" {
		return fmt.Errorf("wecom not configured")
	}
	content := buildWeComText(title, message, level, timeFootnote)
	body, err := json.Marshal(map[string]any{
		"msgtype": "text",
		"text":    map[string]string{"content": content},
	})
	if err != nil {
		return fmt.Errorf("wecom payload marshal: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, cfg.WecomWebhook, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("wecom request build failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := doNotifyRequest(req)
	if err != nil {
		return fmt.Errorf("wecom request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("wecom response status %d", resp.StatusCode)
	}
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("wecom webhook returned invalid JSON")
	}
	if result.ErrCode != 0 {
		return fmt.Errorf("wecom webhook error %d: %s", result.ErrCode, result.ErrMsg)
	}
	return nil
}
