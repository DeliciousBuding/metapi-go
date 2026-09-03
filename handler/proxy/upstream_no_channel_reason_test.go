package proxyhandler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
)

// #1179 — "部署后基本用不了": every routing failure answered with a bare
// "No available channels" and a server log line of `channel selection failed
// err=<nil>`. An operator could not tell an unmatched route from a fleet of
// channels whose tokens were never bound, from cooldown, or from a downstream
// key policy — the three have completely different fixes. The router already
// knows (routing.TokenRouter.ExplainSelection, the same source the route
// decision panel reads); the failure path just never asked.

// explainingRouter is the all-failed stub plus the optional explanation
// capability routing.TokenRouter has in production.
type explainingRouter struct {
	allFailedAlertRouter
	explanation routing.RouteDecisionExplanation
	explainErr  error
	explained   int
}

func (r *explainingRouter) ExplainSelection(_ context.Context, _ string, _ []int64, _ routing.DownstreamRoutingPolicy) (routing.RouteDecisionExplanation, error) {
	r.explained++
	return r.explanation, r.explainErr
}

func TestDispatchNoAvailableChannelsCarriesTheRealReason(t *testing.T) {
	db := setupAllFailedAlertDB(t)
	router := &explainingRouter{
		allFailedAlertRouter: allFailedAlertRouter{}, // selects nothing, no error
		explanation: routing.RouteDecisionExplanation{
			RequestedModel: "gpt-reason",
			ActualModel:    "gpt-reason",
			Matched:        true,
			Summary: []string{
				"命中路由：gpt-*",
				"没有可用通道（全部被禁用、站点不可用、冷却或令牌不可用）",
			},
			Candidates: []routing.RouteDecisionCandidate{
				{ChannelID: 1, Eligible: false, Reason: "令牌不可用"},
				{ChannelID: 2, Eligible: false, Reason: "令牌不可用"},
				{ChannelID: 3, Eligible: false, Reason: "冷却中"},
			},
		},
	}
	SetUpstreamConfig(&UpstreamConfig{Router: router})
	t.Cleanup(func() { SetUpstreamConfig(nil) })

	req := makeProxyReq("POST", "/v1/chat/completions",
		`{"model":"gpt-reason","messages":[{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	HandleChatCompletions(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body = %s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	msg := envelope.Error.Message
	if !strings.Contains(msg, "No available channels") {
		t.Fatalf("message = %q, want it to keep the stable %q prefix", msg, "No available channels")
	}
	// The dominant per-candidate reason is the actionable part: two channels had
	// no usable token, one was cooling down.
	if !strings.Contains(msg, "令牌不可用") {
		t.Fatalf("message = %q, want the dominant candidate reason 令牌不可用", msg)
	}
	if router.explained == 0 {
		t.Fatal("the failure path never asked the router why nothing was selectable")
	}

	// The same reason reaches the operator-facing event feed, not only the wire.
	eventMsg := lastAllFailedProxyEventMessage(t, db)
	if !strings.Contains(eventMsg, "no available channels") {
		t.Fatalf("event message = %q, want the stable reason label", eventMsg)
	}
	if !strings.Contains(eventMsg, "令牌不可用") {
		t.Fatalf("event message = %q, want the explanation too", eventMsg)
	}
}

func TestDispatchNoAvailableChannelsWithoutExplainerKeepsTheOldMessage(t *testing.T) {
	// Unit stubs do not implement SelectionExplainer. A missing reason must
	// degrade to the previous message, never into a harder failure.
	setupAllFailedAlertDB(t)
	SetUpstreamConfig(&UpstreamConfig{Router: &allFailedAlertRouter{}})
	t.Cleanup(func() { SetUpstreamConfig(nil) })

	req := makeProxyReq("POST", "/v1/chat/completions",
		`{"model":"gpt-no-explainer","messages":[{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	HandleChatCompletions(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body = %s", rec.Code, rec.Body.String())
	}
	var envelope struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if envelope.Error.Message != "No available channels" {
		t.Fatalf("message = %q, want the unchanged %q", envelope.Error.Message, "No available channels")
	}
}

func TestExplainNoChannelRendersVerdictAndDominantReason(t *testing.T) {
	cases := []struct {
		name        string
		explanation routing.RouteDecisionExplanation
		explainErr  error
		noExplainer bool
		want        string
	}{
		{
			name:        "no route matched",
			explanation: routing.RouteDecisionExplanation{Summary: []string{"未匹配到启用的路由"}},
			want:        "未匹配到启用的路由",
		},
		{
			name: "route matched, every candidate rejected",
			explanation: routing.RouteDecisionExplanation{
				Summary: []string{"命中路由：gpt-*", "没有可用通道（全部被禁用、站点不可用、冷却或令牌不可用）"},
				Candidates: []routing.RouteDecisionCandidate{
					{Eligible: false, Reason: "冷却中"},
					{Eligible: false, Reason: "令牌不可用"},
					{Eligible: false, Reason: "令牌不可用"},
				},
			},
			want: "没有可用通道（全部被禁用、站点不可用、冷却或令牌不可用）：令牌不可用",
		},
		{
			name: "eligible candidates are not a rejection reason",
			explanation: routing.RouteDecisionExplanation{
				Summary:    []string{"命中路由：gpt-*"},
				Candidates: []routing.RouteDecisionCandidate{{Eligible: true, Reason: ""}},
			},
			want: "命中路由：gpt-*",
		},
		{
			name:       "explanation failed",
			explainErr: context.DeadlineExceeded,
			want:       "",
		},
		{
			name:        "router cannot explain",
			noExplainer: true,
			want:        "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var router proxy.TokenRouterInterface = &explainingRouter{
				explanation: tc.explanation,
				explainErr:  tc.explainErr,
			}
			if tc.noExplainer {
				router = &allFailedAlertRouter{}
			}
			got := proxy.ExplainNoChannel(context.Background(), router, "gpt-x", nil, routing.DownstreamRoutingPolicy{})
			if got != tc.want {
				t.Fatalf("ExplainNoChannel = %q, want %q", got, tc.want)
			}
		})
	}
}
