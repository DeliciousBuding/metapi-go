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
	"net/url"
	"strconv"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

// DingtalkChannel sends notifications via a DingTalk custom-bot webhook
// (https://oapi.dingtalk.com/robot/send?access_token=...).
//
// When DingtalkSecret is set, appends &timestamp=&sign= query params signed
// per the DingTalk custom-bot protocol: HMAC-SHA256(key = secret, message =
// timestamp+"\n"+secret) → base64(raw digest), url-encoded.
// Content uses the DingTalk "text" msgtype (markdown also possible later).
type DingtalkChannel struct{}

func (c *DingtalkChannel) Name() string { return "dingtalk" }

func (c *DingtalkChannel) Send(cfg *config.Config, title, message, level, timeFootnote string) error {
	if !cfg.DingtalkEnabled || cfg.DingtalkWebhook == "" {
		return fmt.Errorf("dingtalk not configured")
	}
	content := buildDingtalkText(title, message, level, timeFootnote)
	body, err := json.Marshal(map[string]any{
		"msgtype": "text",
		"text":    map[string]string{"content": content},
	})
	if err != nil {
		return fmt.Errorf("dingtalk payload marshal: %w", err)
	}

	endpoint := cfg.DingtalkWebhook
	if cfg.DingtalkSecret != "" {
		ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
		sign := dingtalkSign(ts, cfg.DingtalkSecret)
		sep := "&"
		if !containsQuery(endpoint) {
			sep = "?"
		}
		endpoint = fmt.Sprintf("%s%stimestamp=%s&sign=%s", endpoint, sep, ts, url.QueryEscape(sign))
	}

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("dingtalk request build failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := doNotifyRequest(req)
	if err != nil {
		return fmt.Errorf("dingtalk request failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("dingtalk response status %d", resp.StatusCode)
	}
	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg   string `json:"errmsg"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("dingtalk webhook returned invalid JSON")
	}
	if result.ErrCode != 0 {
		return fmt.Errorf("dingtalk webhook error %d: %s", result.ErrCode, result.ErrMsg)
	}
	return nil
}

// dingtalkSign computes the DingTalk custom-bot signature: HMAC-SHA256
// (key = secret, message = timestamp+"\n"+secret) → base64(raw digest).
func dingtalkSign(timestamp, secret string) string {
	stringToSign := timestamp + "\n" + secret
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func buildDingtalkText(title, message, level, timeFootnote string) string {
	const maxLen = 1800 // DingTalk text msg content cap is ~20000 chars; we keep terse
	raw := fmt.Sprintf("[metapi][%s] %s\n\n%s\n\n%s", upperLevel(level), title, message, timeFootnote)
	if len(raw) <= maxLen {
		return raw
	}
	return raw[:maxLen] + "\n...(truncated)"
}

func upperLevel(level string) string {
	out := make([]byte, 0, len(level))
	for i := 0; i < len(level); i++ {
		c := level[i]
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		out = append(out, c)
	}
	return string(out)
}

func containsQuery(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return u.RawQuery != ""
}
