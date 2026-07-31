package notify

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/tokendancelab/metapi-go/config"
)

// FeishuChannel sends notifications via a Feishu/Lark custom bot webhook
// (https://open.feishu.cn/open-apis/bot/v2/hook/<id>). All-api-hub borrow D1.
//
// When FeishuSecret is set, signs the request timestamp with HMAC-SHA256
// (secret = key) per the Feishu bot sign protocol so the bot verifies origin.
// Reuses buildFeishuText from webhook.go for the content body (max-length
// aware), so text rendering stays consistent with the generic webhook path.
type FeishuChannel struct{}

func (c *FeishuChannel) Name() string { return "feishu" }

func (c *FeishuChannel) Send(cfg *config.Config, title, message, level, timeFootnote string) error {
	if !cfg.FeishuEnabled || cfg.FeishuWebhook == "" {
		return fmt.Errorf("feishu not configured")
	}
	content := buildFeishuText(title, message, level, timeFootnote)
	payload := map[string]any{
		"msg_type": "text",
		"content":  map[string]string{"text": content},
	}
	if cfg.FeishuSecret != "" {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		payload["timestamp"] = ts
		payload["sign"] = feishuSign(ts, cfg.FeishuSecret)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("feishu payload marshal: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, cfg.FeishuWebhook, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("feishu request build failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := doNotifyRequest(req)
	if err != nil {
		return fmt.Errorf("feishu request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("feishu response status %d", resp.StatusCode)
	}
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("feishu webhook returned invalid JSON")
	}
	if result.Code != 0 {
		return fmt.Errorf("feishu webhook error %d: %s", result.Code, result.Msg)
	}
	return nil
}

// feishuSign computes the Feishu bot signature per the documented custom-bot
// sign protocol: HMAC-SHA256(key = timestamp+"\n"+secret, message = "") →
// base64(raw digest). See Feishu open platform "custom bot" sign docs.
func feishuSign(timestamp, secret string) string {
	stringToSign := timestamp + "\n" + secret
	mac := hmac.New(sha256.New, []byte(stringToSign))
	mac.Write([]byte{})
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
