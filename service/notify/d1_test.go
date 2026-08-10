package notify

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

// roundTripFunc is shared with http_test.go (same package).
// newTestClient swaps the package notifyHTTPClient with a capturing transport.
func newTestClient(t *testing.T, capture *[]*http.Request, respBody string, status int) *http.Client {
	t.Helper()
	old := notifyHTTPClient
	t.Cleanup(func() { notifyHTTPClient = old })
	c := &http.Client{
		Timeout: notifyHTTPTimeout,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			*capture = append(*capture, req)
			return &http.Response{
				StatusCode: status,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(respBody)),
				Request:    req,
			}, nil
		}),
	}
	notifyHTTPClient = c
	return c
}

// ---- Signature tests (deterministic) ----

func TestFeishuSignDeterministic(t *testing.T) {
	a := feishuSign("1700000000", "mysecret")
	b := feishuSign("1700000000", "mysecret")
	if a != b {
		t.Fatalf("feishuSign not deterministic: %s != %s", a, b)
	}
	// Different inputs → different outputs.
	c := feishuSign("1700000001", "mysecret")
	if a == c {
		t.Fatal("feishuSign should change when timestamp changes")
	}
	if a == feishuSign("1700000000", "other") {
		t.Fatal("feishuSign should change when secret changes")
	}
}

func TestDingtalkSignDeterministic(t *testing.T) {
	a := dingtalkSign("1700000000000", "mysecret")
	b := dingtalkSign("1700000000000", "mysecret")
	if a != b {
		t.Fatalf("dingtalkSign not deterministic: %s != %s", a, b)
	}
	if a == dingtalkSign("1700000000001", "mysecret") {
		t.Fatal("dingtalkSign should change when timestamp changes")
	}
}

// ---- ntfy send test ----

func TestNtfyChannelSendHeadersAndBody(t *testing.T) {
	var reqs []*http.Request
	newTestClient(t, &reqs, `{"ok":true}`, http.StatusOK)

	cfg := &config.Config{
		NtfyEnabled: true,
		NtfyUrl:     "https://ntfy.example.test",
		NtfyTopic:   "metapi-alerts",
		NtfyToken:   "tok123",
	}
	if err := (&NtfyChannel{}).Send(cfg, "余额不足", " acct-x balance 0.3", "warning", "footnote"); err != nil {
		t.Fatalf("ntfy Send: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	r := reqs[0]
	if r.URL.Path != "/metapi-alerts" {
		t.Fatalf("path = %s, want /metapi-alerts", r.URL.Path)
	}
	if got := r.Header.Get("Title"); !strings.Contains(got, "余额不足") {
		t.Fatalf("Title header = %q", got)
	}
	if got := r.Header.Get("Priority"); got != "high" {
		t.Fatalf("Priority = %q, want high", got)
	}
	if got := r.Header.Get("Authorization"); got != "Bearer tok123" {
		t.Fatalf("Authorization = %q, want Bearer tok123", got)
	}
}

func TestNtfyChannelSkipsWhenDisabled(t *testing.T) {
	var reqs []*http.Request
	newTestClient(t, &reqs, `{"ok":true}`, http.StatusOK)
	cfg := &config.Config{NtfyEnabled: false}
	err := (&NtfyChannel{}).Send(cfg, "t", "m", "info", "f")
	if err == nil {
		t.Fatal("disabled ntfy should error")
	}
	if len(reqs) != 0 {
		t.Fatalf("disabled ntfy should not dial, got %d requests", len(reqs))
	}
}

// ---- Feishu send test (sign included) ----

func TestFeishuChannelSendsWithSign(t *testing.T) {
	var reqs []*http.Request
	newTestClient(t, &reqs, `{"code":0,"msg":"ok"}`, http.StatusOK)
	cfg := &config.Config{
		FeishuEnabled: true,
		FeishuWebhook: "https://open.feishu.cn/open-apis/bot/v2/hook/test",
		FeishuSecret:  "sec",
	}
	if err := (&FeishuChannel{}).Send(cfg, "T", "M", "warning", "F"); err != nil {
		t.Fatalf("feishu Send: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	// Body should contain "sign" + "timestamp" fields (secret configured).
	body, _ := io.ReadAll(reqs[0].Body)
	if !strings.Contains(string(body), `"sign"`) || !strings.Contains(string(body), `"timestamp"`) {
		t.Fatalf("feishu body missing sign/timestamp: %s", body)
	}
}

// ---- DingTalk send test (sign appended to URL) ----

func TestDingtalkChannelSendsWithSignQuery(t *testing.T) {
	var reqs []*http.Request
	newTestClient(t, &reqs, `{"errcode":0,"errmsg":"ok"}`, http.StatusOK)
	cfg := &config.Config{
		DingtalkEnabled: true,
		DingtalkWebhook: "https://oapi.dingtalk.com/robot/send?access_token=abc",
		DingtalkSecret:  "sec",
	}
	if err := (&DingtalkChannel{}).Send(cfg, "T", "M", "error", "F"); err != nil {
		t.Fatalf("dingtalk Send: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	q := reqs[0].URL.Query()
	if q.Get("timestamp") == "" || q.Get("sign") == "" {
		t.Fatalf("dingtalk URL missing timestamp/sign query: %s", reqs[0].URL.String())
	}
}

// ---- WeCom send test ----

func TestWecomChannelSend(t *testing.T) {
	var reqs []*http.Request
	newTestClient(t, &reqs, `{"errcode":0,"errmsg":"ok"}`, http.StatusOK)
	cfg := &config.Config{
		WecomEnabled: true,
		WecomWebhook: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=x",
	}
	if err := (&WecomChannel{}).Send(cfg, "T", "M", "info", "F"); err != nil {
		t.Fatalf("wecom Send: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
}

// ---- Per-task mute gate ----

func TestSendNotificationTaskMuteGateSkipsMutedTask(t *testing.T) {
	var reqs []*http.Request
	newTestClient(t, &reqs, `{"errcode":0}`, http.StatusOK)

	cfg := &config.Config{
		BarkEnabled:         true,
		BarkUrl:             "https://bark.example/test",
		NotifyTaskToggles:   map[string]bool{"low_balance": false},
	}
	res, err := SendNotification(cfg, "余额不足", "m", "warning",
		&SendNotificationOptions{TaskTag: "low_balance"})
	if err != nil {
		t.Fatalf("SendNotification: %v", err)
	}
	if res.Attempted != 0 {
		t.Fatalf("muted task should not attempt, got Attempted=%d", res.Attempted)
	}
	if len(reqs) != 0 {
		t.Fatalf("muted task should not dial, got %d requests", len(reqs))
	}
}

func TestSendNotificationTaskMuteGateAllowsUnmutedTask(t *testing.T) {
	var reqs []*http.Request
	newTestClient(t, &reqs, `{"errcode":0}`, http.StatusOK)

	cfg := &config.Config{
		BarkEnabled:       true,
		BarkUrl:           "https://bark.example/test",
		NotifyTaskToggles: map[string]bool{"low_balance": false},
	}
	res, err := SendNotification(cfg, "Token 已失效", "m", "error",
		&SendNotificationOptions{TaskTag: "token_expired"})
	if err != nil {
		t.Fatalf("SendNotification: %v", err)
	}
	if res.Attempted == 0 {
		t.Fatal("unmuted token_expired task should attempt dispatch")
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request (bark), got %d", len(reqs))
	}
}

func TestSendNotificationNoGateWhenTaskTagEmpty(t *testing.T) {
	// Empty TaskTag = no gating (backward-compatible), even if toggles map has entries.
	var reqs []*http.Request
	newTestClient(t, &reqs, `{"errcode":0}`, http.StatusOK)
	cfg := &config.Config{
		BarkEnabled:       true,
		BarkUrl:           "https://bark.example/test",
		NotifyTaskToggles: map[string]bool{"low_balance": false},
	}
	res, err := SendNotification(cfg, "t", "m", "info", &SendNotificationOptions{})
	if err != nil {
		t.Fatalf("SendNotification: %v", err)
	}
	if res.Attempted == 0 {
		t.Fatal("empty TaskTag should not gate dispatch")
	}
}
