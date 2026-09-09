package proxyhandler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

// Deterministic owned IDs are fixture-only; production uses crypto/rand.
func messagesReplayFixtureIDs(ids ...string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		if id == "" || strings.HasPrefix(id, messagesBridgeToolIDPrefix) {
			out[i] = id
		} else {
			digest := sha256.Sum256([]byte(id))
			out[i] = messagesBridgeToolIDPrefix + hex.EncodeToString(digest[:16])
		}
	}
	return out
}

func messagesReplayFixture(t *testing.T, prompt string) (*http.Request, *Ctx, *routing.SelectedChannel) {
	t.Helper()
	keyID, tokenID := int64(7), int64(44)
	ctx := &Ctx{
		Auth:           &auth.ProxyAuthContext{Source: "managed", KeyID: &keyID, Token: "fixture-downstream-credential"},
		RequestedModel: "bridge-model",
		Body: map[string]any{
			"model": "bridge-model", "max_tokens": 32000,
			"thinking":      map[string]any{"type": "adaptive", "display": "omitted"},
			"output_config": map[string]any{"effort": "high"},
			"metadata":      map[string]any{"user_id": `{"device_id":"fixture-device","session_id":"fixture-session"}`},
			"system":        "Use the Read tool.",
			"messages":      []any{map[string]any{"role": "user", "content": prompt}},
			"tools":         []any{map[string]any{"name": "Read", "input_schema": map[string]any{"type": "object"}}},
		},
	}
	ctx.ClientCtx.SessionID = "profile-session"
	messagesReplayRefresh(t, ctx)
	selected := &routing.SelectedChannel{
		Site:    store.Site{ID: 11, URL: "https://upstream.invalid", Platform: "openai"},
		Account: store.Account{ID: 22}, Channel: store.RouteChannel{ID: 33, TokenID: &tokenID},
		Token: &store.AccountToken{ID: tokenID}, TokenValue: "fixture-upstream-credential", ActualModel: "relay-model",
	}
	r := httptest.NewRequest(http.MethodPost, "http://gateway.invalid/v1/messages", nil)
	return r, ctx, selected
}

func messagesReplayRefresh(t *testing.T, ctx *Ctx) {
	t.Helper()
	body, err := json.Marshal(ctx.Body)
	if err != nil {
		t.Fatal(err)
	}
	ctx.RawBody = body
}

func messagesReplayFor(cache *messagesReplayCache, r *http.Request, ctx *Ctx, selected *routing.SelectedChannel) *messagesBridgeRequest {
	request := newMessagesBridgeRequest(r, ctx, selected)
	request.cache = cache
	return request
}

func messagesReplayAppend(t *testing.T, ctx *Ctx, ids ...string) {
	t.Helper()
	ids = messagesReplayFixtureIDs(ids...)
	var calls, results []any
	for _, id := range ids {
		calls = append(calls, map[string]any{"type": "tool_use", "id": id, "name": "Read", "input": map[string]any{"file_path": id + ".txt"}})
		results = append(results, map[string]any{"type": "tool_result", "tool_use_id": id, "content": "fixture file text"})
	}
	ctx.Body["messages"] = append(ctx.Body["messages"].([]any),
		map[string]any{"role": "assistant", "content": calls},
		map[string]any{"role": "user", "content": results})
	messagesReplayRefresh(t, ctx)
}

func messagesReplayResponse(t *testing.T, reasoning string, ids ...string) []byte {
	t.Helper()
	var calls []any
	for _, id := range ids {
		arguments, _ := json.Marshal(map[string]any{"file_path": id + ".txt"})
		calls = append(calls, map[string]any{"id": id, "type": "function", "function": map[string]any{"name": "Read", "arguments": string(arguments)}})
	}
	value := map[string]any{"id": "chatcmpl-fixture", "model": "relay-model", "choices": []any{map[string]any{
		"index": 0, "finish_reason": "tool_calls", "message": map[string]any{"role": "assistant", "content": nil, "reasoning_content": reasoning, "tool_calls": calls},
	}}}
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func messagesReplaySave(t *testing.T, cache *messagesReplayCache, prompt, reasoning string, ids ...string) {
	t.Helper()
	r, ctx, selected := messagesReplayFixture(t, prompt)
	request := messagesReplayFor(cache, r, ctx, selected)
	if err := request.Options().SaveReasoning(messagesReplayFixtureIDs(ids...), reasoning); err != nil {
		t.Fatal(err)
	}
	if request.UsedReplay() {
		t.Fatal("Save marked the request as having loaded replay")
	}
}

func messagesReplayLoad(t *testing.T, cache *messagesReplayCache, prompt string, ids ...string) (string, bool) {
	t.Helper()
	r, ctx, selected := messagesReplayFixture(t, prompt)
	messagesReplayAppend(t, ctx, ids...)
	request := messagesReplayFor(cache, r, ctx, selected)
	value, found, err := request.Options().LoadReasoning(messagesReplayFixtureIDs(ids...))
	if err != nil && (!errors.Is(err, messages.ErrReasoningReplay) || found) {
		t.Fatal(err)
	}
	if request.UsedReplay() != found {
		t.Fatalf("UsedReplay=%v, found=%v", request.UsedReplay(), found)
	}
	return value, found
}

func TestMessagesReplayCallerOptionsRoundTrip(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	r, ctx, selected := messagesReplayFixture(t, "Read both files.")
	request := messagesReplayFor(cache, r, ctx, selected)
	options := request.Options()
	if _, err := messages.ToChatRequest(ctx.RawBody, options); err != nil {
		t.Fatal(err)
	}
	if request.UsedReplay() {
		t.Fatal("first native request should not be pinned to Chat by replay")
	}
	const hidden = "fixture reasoning retained only for tool continuation"
	reply, err := messages.FromChatResponse(messagesReplayResponse(t, hidden, "a", "b"), options)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(reply, []byte(hidden)) || bytes.Contains(reply, []byte("reasoning_content")) {
		t.Fatal("hidden reasoning leaked to the native client")
	}
	messagesReplayAppendResponse(t, ctx, reply)
	messagesReplayExpectChannel(t, cache, ctx, selected.Channel.ID)
	next := messagesReplayFor(cache, r, ctx, selected)
	if next.UsedReplay() {
		t.Fatal("replay was marked before Load")
	}
	chat, err := messages.ToChatRequest(ctx.RawBody, next.Options())
	if err != nil {
		t.Fatal(err)
	}
	if !next.UsedReplay() {
		t.Fatal("successful replay did not select Chat-only continuation")
	}
	var value map[string]any
	if err := json.Unmarshal(chat, &value); err != nil {
		t.Fatal(err)
	}
	if value["reasoning_effort"] != "high" {
		t.Fatal("thinking was disabled")
	}
	found := false
	for _, item := range value["messages"].([]any) {
		message := item.(map[string]any)
		if message["role"] == "assistant" {
			found = true
			if message["reasoning_content"] != hidden || len(message["tool_calls"].([]any)) != 2 {
				t.Fatal("reasoning or complete tool group lost in the next Chat request")
			}
		}
	}
	if !found {
		t.Fatal("assistant history missing")
	}
}

func TestMessagesReplayIsolation(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	messagesReplaySave(t, cache, "same prompt", "private fixture reasoning", "call_0")
	for _, tc := range []struct {
		name   string
		mutate func(*Ctx, *routing.SelectedChannel)
	}{
		{"downstream token", func(c *Ctx, _ *routing.SelectedChannel) { c.Auth.Token = "different-downstream-credential" }},
		{"downstream key row", func(c *Ctx, _ *routing.SelectedChannel) { *c.Auth.KeyID = 8 }},
		{"authentication source", func(c *Ctx, _ *routing.SelectedChannel) { c.Auth.Source = "global" }},
		{"profile session", func(c *Ctx, _ *routing.SelectedChannel) { c.ClientCtx.SessionID = "other-session" }},
		{"opaque metadata session", func(c *Ctx, _ *routing.SelectedChannel) {
			c.Body["metadata"].(map[string]any)["user_id"] = "other-opaque-session"
		}},
		{"requested model", func(c *Ctx, _ *routing.SelectedChannel) {
			c.RequestedModel = "another-requested-model"
			c.Body["model"] = c.RequestedModel
		}},
		{"upstream model", func(_ *Ctx, s *routing.SelectedChannel) { s.ActualModel = "another-upstream-model" }},
		{"upstream account", func(_ *Ctx, s *routing.SelectedChannel) { s.Account.ID++ }},
		{"upstream site", func(_ *Ctx, s *routing.SelectedChannel) { s.Site.ID++ }},
		{"upstream URL", func(_ *Ctx, s *routing.SelectedChannel) { s.Site.URL = "https://different-upstream.invalid" }},
		{"upstream platform", func(_ *Ctx, s *routing.SelectedChannel) { s.Site.Platform = "other-platform" }},
		{"upstream channel", func(_ *Ctx, s *routing.SelectedChannel) { s.Channel.ID++ }},
		{"upstream token row", func(_ *Ctx, s *routing.SelectedChannel) { s.Token.ID++ }},
		{"upstream credential", func(_ *Ctx, s *routing.SelectedChannel) { s.TokenValue = "different-upstream-credential" }},
		{"transcript prefix", func(c *Ctx, _ *routing.SelectedChannel) {
			c.Body["messages"].([]any)[0].(map[string]any)["content"] = "different prompt"
		}},
		{"system prefix", func(c *Ctx, _ *routing.SelectedChannel) { c.Body["system"] = "Different system instruction." }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r, ctx, selected := messagesReplayFixture(t, "same prompt")
			messagesReplayAppend(t, ctx, "call_0")
			tc.mutate(ctx, selected)
			messagesReplayRefresh(t, ctx)
			request := messagesReplayFor(cache, r, ctx, selected)
			out, err := messages.ToChatRequest(ctx.RawBody, request.Options())
			if !errors.Is(err, messages.ErrReasoningReplay) || len(out) != 0 || request.UsedReplay() {
				t.Fatalf("cross-scope replay was allowed (error=%v, UsedReplay=%v)", err, request.UsedReplay())
			}
		})
	}
}

func TestMessagesReplayGlobalKeysAndOpaqueSession(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	r, ctx, selected := messagesReplayFixture(t, "read")
	ctx.Auth.Source, ctx.Auth.KeyID = "global", nil
	ctx.ClientCtx.SessionID = "" // Real JSON user_id does not match the legacy detector.
	request := messagesReplayFor(cache, r, ctx, selected)
	if err := request.Options().SaveReasoning(messagesReplayFixtureIDs("call_0"), "hidden"); err != nil {
		t.Fatal(err)
	}
	messagesReplayAppend(t, ctx, "call_0")
	r.Header.Set("Authorization", "Bearer untrusted-header-value")
	next := messagesReplayFor(cache, r, ctx, selected)
	if _, err := messages.ToChatRequest(ctx.RawBody, next.Options()); err != nil || !next.UsedReplay() {
		t.Fatalf("verified auth/opaque session was replaced by raw header identity: %v", err)
	}
	ctx.Auth.Token = "different-global-key"
	next = messagesReplayFor(cache, r, ctx, selected)
	if _, err := messages.ToChatRequest(ctx.RawBody, next.Options()); !errors.Is(err, messages.ErrReasoningReplay) || next.UsedReplay() {
		t.Fatalf("two global keys shared replay: %v", err)
	}
}

func TestMessagesReplayCompleteOrderedGroup(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	messagesReplaySave(t, cache, "read", "hidden", "a", "b")
	for _, ids := range [][]string{{"a"}, {"b", "a"}, {"a", "b", "c"}, {"a", "a"}, {""}, nil} {
		r, ctx, selected := messagesReplayFixture(t, "read")
		messagesReplayAppend(t, ctx, "a", "b")
		request := messagesReplayFor(cache, r, ctx, selected)
		_, found, err := request.Options().LoadReasoning(messagesReplayFixtureIDs(ids...))
		if found || err == nil || request.UsedReplay() {
			t.Fatal("partial/reordered/invalid tool group read a record")
		}
	}
	if value, found := messagesReplayLoad(t, cache, "read", "a", "b"); !found || value != "hidden" {
		t.Fatal("complete ordered group did not load")
	}
}

func TestMessagesReplayReusedIDsHaveDifferentPrefixes(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	messagesReplaySave(t, cache, "branch A", "reason A", "call_0")
	messagesReplaySave(t, cache, "branch B", "reason B", "call_0")
	for _, branch := range []string{"A", "B"} {
		value, found := messagesReplayLoad(t, cache, "branch "+branch, "call_0")
		if !found || value != "reason "+branch {
			t.Fatal("same-session reused tool ID selected another transcript prefix")
		}
	}
	// The callback contract also disambiguates repeated ordered groups in one
	// history, rather than using an ID -> latest-record map.
	r, ctx, selected := messagesReplayFixture(t, "branch A")
	messagesReplayAppend(t, ctx, "call_0")
	second := messagesReplayFor(cache, r, ctx, selected)
	if err := second.Options().SaveReasoning(messagesReplayFixtureIDs("call_0"), "next turn"); err != nil {
		t.Fatal(err)
	}
	messagesReplayAppend(t, ctx, "call_0")
	third := messagesReplayFor(cache, r, ctx, selected)
	options := third.Options()
	for _, want := range []string{"reason A", "next turn"} {
		value, found, err := options.LoadReasoning(messagesReplayFixtureIDs("call_0"))
		if err != nil || !found || value != want {
			t.Fatal("reused ID occurrences were not matched to their historical prefixes")
		}
	}
	if _, found, err := options.LoadReasoning(messagesReplayFixtureIDs("call_0")); found || err == nil {
		t.Fatal("Load walked beyond request history")
	}
	// A fresh conversion gets a fresh cursor, not a partially consumed one.
	if value, found, err := third.Options().LoadReasoning(messagesReplayFixtureIDs("call_0")); err != nil || !found || value != "reason A" {
		t.Fatal("fresh Options did not reset the conversion cursor")
	}
}

func TestMessagesReplayKnownEmptyAndMissingAreDistinct(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	messagesReplaySave(t, cache, "read", "", "call_0")
	r, ctx, selected := messagesReplayFixture(t, "read")
	messagesReplayAppend(t, ctx, "call_0")
	request := messagesReplayFor(cache, r, ctx, selected)
	out, err := messages.ToChatRequest(ctx.RawBody, request.Options())
	if err != nil || !request.UsedReplay() || bytes.Contains(out, []byte("reasoning_content")) {
		t.Fatalf("known-empty replay was missing or fabricated reasoning: %v", err)
	}
	missing := messagesReplayFor(newMessagesReplayCache(), r, ctx, selected)
	if out, err := messages.ToChatRequest(ctx.RawBody, missing.Options()); !errors.Is(err, messages.ErrReasoningReplay) || len(out) != 0 || missing.UsedReplay() {
		t.Fatal("missing replay looked like a known-empty record")
	}
}

func TestMessagesReplayAbsoluteTTL(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	now := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	cache.now = func() time.Time { return now }
	messagesReplaySave(t, cache, "read", "hidden", "a")
	now = now.Add(messagesReplayTTL - time.Nanosecond)
	if _, found := messagesReplayLoad(t, cache, "read", "a"); !found {
		t.Fatal("record expired too soon")
	}
	now = now.Add(time.Nanosecond)
	if _, found := messagesReplayLoad(t, cache, "read", "a"); found {
		t.Fatal("an LRU read extended absolute TTL")
	}
	if len(cache.entries) != 0 || cache.bytes != 0 || cache.lru.Len() != 0 {
		t.Fatal("expired record retained cache capacity")
	}
	messagesReplaySave(t, cache, "read", "new reasoning", "a")
	if value, found := messagesReplayLoad(t, cache, "read", "a"); !found || value != "new reasoning" {
		t.Fatal("expired record was not replaceable")
	}
}

func TestMessagesReplayLRUAndByteCapacity(t *testing.T) {
	t.Parallel()
	for _, byBytes := range []bool{false, true} {
		t.Run(fmt.Sprint(byBytes), func(t *testing.T) {
			cache := newMessagesReplayCache()
			if byBytes {
				cache.maxEntries = 10
				cache.maxBytes = 2 * (messagesReplayEntryCost + 4)
			} else {
				cache.maxEntries = 2
			}
			messagesReplaySave(t, cache, "A", "1111", "call_0")
			messagesReplaySave(t, cache, "B", "2222", "call_0")
			if _, found := messagesReplayLoad(t, cache, "A", "call_0"); !found {
				t.Fatal("first record missing before capacity pressure")
			}
			messagesReplaySave(t, cache, "C", "3333", "call_0")
			if _, found := messagesReplayLoad(t, cache, "B", "call_0"); found {
				t.Fatal("least-recently-used record was not evicted")
			}
			for _, prompt := range []string{"A", "C"} {
				if _, found := messagesReplayLoad(t, cache, prompt, "call_0"); !found {
					t.Fatal("LRU evicted the wrong record")
				}
			}
			if len(cache.entries) != 2 || cache.bytes != 2*(messagesReplayEntryCost+4) || cache.bytes > cache.maxBytes {
				t.Fatal("entry/byte accounting is not bounded")
			}
		})
	}
}

func TestMessagesReplayRecordLimitFailsBeforeTerminal(t *testing.T) {
	t.Parallel()
	for _, limit := range []string{"record bytes", "total bytes", "entries"} {
		t.Run(limit, func(t *testing.T) {
			cache := newMessagesReplayCache()
			cache.maxRecordBytes = 6
			if limit == "total bytes" {
				cache.maxRecordBytes = 100
				cache.maxBytes = messagesReplayEntryCost + 6
			}
			if limit == "entries" {
				cache.maxEntries = 0
			}
			r, ctx, selected := messagesReplayFixture(t, "read")
			request := messagesReplayFor(cache, r, ctx, selected)
			out, err := messages.FromChatResponse(messagesReplayResponse(t, "too big", "a"), request.Options())
			if !errors.Is(err, messages.ErrReasoningReplay) || len(out) != 0 || len(cache.entries) != 0 || cache.bytes != 0 {
				t.Fatal("capacity failure released a successful response or retained data")
			}
		})
	}
	cache := newMessagesReplayCache()
	messagesReplaySave(t, cache, "at limit", strings.Repeat("a", messagesReplayMaxRecord), "a")
	r, ctx, selected := messagesReplayFixture(t, "over limit")
	request := messagesReplayFor(cache, r, ctx, selected)
	if err := request.Options().SaveReasoning(messagesReplayFixtureIDs("b"), strings.Repeat("b", messagesReplayMaxRecord+1)); !errors.Is(err, messages.ErrReasoningReplay) || len(cache.entries) != 1 {
		t.Fatal("single-record hard limit was not enforced before eviction/write")
	}
}

func TestMessagesReplayMissingIdentityCannotBeAuthenticatedByMetadata(t *testing.T) {
	t.Parallel()
	for _, change := range []func(*Ctx, *routing.SelectedChannel){
		func(c *Ctx, _ *routing.SelectedChannel) { c.Auth = nil },
		func(c *Ctx, _ *routing.SelectedChannel) { c.Auth.Source = "unknown" },
		func(c *Ctx, _ *routing.SelectedChannel) { c.Auth.Source = "global"; c.Auth.Token = "" },
		func(c *Ctx, _ *routing.SelectedChannel) { c.Auth.KeyID = nil; c.Auth.Token = "" },
		func(c *Ctx, _ *routing.SelectedChannel) { c.ClientCtx.SessionID = ""; delete(c.Body, "metadata") },
		func(c *Ctx, _ *routing.SelectedChannel) {
			c.ClientCtx.SessionID = " "
			c.Body["metadata"] = map[string]any{"user_id": " "}
		},
		func(_ *Ctx, s *routing.SelectedChannel) { s.Channel.ID = 0 },
		func(_ *Ctx, s *routing.SelectedChannel) { s.Account.ID = 0; s.TokenValue = "" },
		func(_ *Ctx, s *routing.SelectedChannel) { s.Site.ID = 0; s.Site.URL = "" },
		func(c *Ctx, s *routing.SelectedChannel) { c.RequestedModel = ""; s.ActualModel = "" },
	} {
		cache := newMessagesReplayCache()
		r, ctx, selected := messagesReplayFixture(t, "read")
		r.Header.Set("Authorization", "Bearer fixture-downstream-credential")
		r.Header.Set("X-Request-ID", "not-a-conversation")
		change(ctx, selected)
		request := messagesReplayFor(cache, r, ctx, selected)
		err := request.Options().SaveReasoning(messagesReplayFixtureIDs("a"), "")
		if !errors.Is(err, messages.ErrReasoningReplay) || len(cache.entries) != 0 || request.UsedReplay() {
			t.Fatal("missing verified scope was silently accepted")
		}
		for _, secret := range []string{"fixture-downstream-credential", "fixture-upstream-credential", "fixture-session"} {
			if strings.Contains(err.Error(), secret) {
				t.Fatal("scope error exposed sensitive identity material")
			}
		}
	}
}

func TestMessagesReplayConflictAndHashedKeys(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	messagesReplaySave(t, cache, "read", "original", "a")
	messagesReplaySave(t, cache, "read", "original", "a")
	r, ctx, selected := messagesReplayFixture(t, "read")
	request := messagesReplayFor(cache, r, ctx, selected)
	if err := request.Options().SaveReasoning(messagesReplayFixtureIDs("a"), "conflicting"); !errors.Is(err, messages.ErrReasoningReplay) {
		t.Fatal("same prefix/group silently replaced another reasoning record")
	}
	if value, found := messagesReplayLoad(t, cache, "read", "a"); !found || value != "original" {
		t.Fatal("failed save damaged the previous valid record")
	}
	if len(cache.entries) != 1 || cache.bytes != messagesReplayEntryCost+len("original") {
		t.Fatal("idempotent/conflicting save leaked entry capacity")
	}
	for key := range cache.entries {
		keyBytes := append(append(append(append([]byte{}, key.client[:]...), key.scope[:]...), key.prefix[:]...), key.group[:]...)
		for _, value := range []string{ctx.Auth.Token, selected.TokenValue, ctx.Body["metadata"].(map[string]any)["user_id"].(string), "original"} {
			if bytes.Contains(keyBytes, []byte(value)) {
				t.Fatal("cache key retained plaintext sensitive content")
			}
		}
	}
}

func TestMessagesReplayConcurrentLoadSaveAndEviction(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	cache.maxEntries, cache.maxBytes = 8, 8*(messagesReplayEntryCost+8)
	const workers = 24
	type fixture struct {
		save, load *messagesBridgeRequest
		reasoning  string
		ctx        *Ctx
		channelID  int64
	}
	fixtures := make([]fixture, workers)
	for i := range workers {
		r, ctx, selected := messagesReplayFixture(t, fmt.Sprintf("prompt-%d", i))
		selected.Channel.ID += int64(i)
		save := messagesReplayFor(cache, r, ctx, selected)
		messagesReplayAppend(t, ctx, "call_0")
		fixtures[i] = fixture{save: save, load: messagesReplayFor(cache, r, ctx, selected), reasoning: fmt.Sprintf("r-%02d", i), ctx: ctx, channelID: selected.Channel.ID}
	}
	var wg sync.WaitGroup
	failures := make(chan string, workers)
	for _, f := range fixtures {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				if err := f.save.Options().SaveReasoning(messagesReplayFixtureIDs("call_0"), f.reasoning); err != nil {
					failures <- "unexpected concurrent Save failure"
					return
				}
				id, required, err := cache.preferredChannel(f.ctx)
				if !required || (err == nil && (id == nil || *id != f.channelID)) || (err != nil && (id != nil || !errors.Is(err, messages.ErrReasoningReplay))) {
					failures <- "concurrent affinity crossed a prefix or lost the required marker"
					return
				}
				got, found, err := f.load.Options().LoadReasoning(messagesReplayFixtureIDs("call_0"))
				if (found && (err != nil || got != f.reasoning)) || (!found && !errors.Is(err, messages.ErrReasoningReplay)) {
					failures <- "concurrent replay crossed a prefix or failed"
					return
				}
				_ = f.load.UsedReplay()
			}
		}()
	}
	wg.Wait()
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}
	if len(cache.entries) > cache.maxEntries || cache.bytes > cache.maxBytes || cache.lru.Len() != len(cache.entries) {
		t.Fatal("concurrent eviction violated cache bounds")
	}
}

func TestMessagesReplayCanceledRequestCannotSave(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	r, ctx, selected := messagesReplayFixture(t, "read")
	requestCtx, cancel := context.WithCancel(r.Context())
	request := messagesReplayFor(cache, r.WithContext(requestCtx), ctx, selected)
	cancel()
	if err := request.Options().SaveReasoning(messagesReplayFixtureIDs("a"), "hidden"); !errors.Is(err, messages.ErrReasoningReplay) || len(cache.entries) != 0 {
		t.Fatal("canceled request saved a successful continuation")
	}
}

func TestMessagesReplayStreamSaveFailureDoesNotReleaseTerminal(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	cache.maxRecordBytes = 3
	r, ctx, selected := messagesReplayFixture(t, "read")
	request := messagesReplayFor(cache, r, ctx, selected)
	stream := messages.NewChatStream("relay-model", request.Options())
	frames := []string{
		`data: {"id":"chatcmpl-fixture","model":"relay-model","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"private"},"finish_reason":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-fixture","model":"relay-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"a","type":"function","function":{"name":"Read","arguments":"{}"}}]},"finish_reason":null}]}` + "\n\n",
		`data: {"id":"chatcmpl-fixture","model":"relay-model","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n",
	}
	var visible bytes.Buffer
	for _, frame := range frames {
		out, err := stream.TransformEvent([]byte(frame))
		if err != nil {
			t.Fatal(err)
		}
		visible.Write(out)
	}
	out, err := stream.TransformEvent([]byte("data: [DONE]\n\n"))
	visible.Write(out)
	if !errors.Is(err, messages.ErrReasoningReplay) || len(out) != 0 || len(cache.entries) != 0 {
		t.Fatal("uncacheable reasoning released a successful stream terminal")
	}
	for _, forbidden := range []string{"private", "event: message_stop", "event: message_delta"} {
		if strings.Contains(visible.String(), forbidden) {
			t.Fatal("failed capture leaked hidden reasoning or reported native success")
		}
	}
	if _, err := stream.Finish(); !errors.Is(err, messages.ErrReasoningReplay) {
		t.Fatal("EOF erased the replay Save failure")
	}
}

func TestMessagesReplayHintsAndStringTextNormalization(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	r, ctx, selected := messagesReplayFixture(t, "read")
	ctx.Body["system"] = []any{map[string]any{"type": "text", "text": "Use the Read tool.", "cache_control": map[string]any{"type": "ephemeral", "ttl": "5m"}}}
	ctx.Body["messages"].([]any)[0].(map[string]any)["content"] = []any{map[string]any{"type": "text", "text": "read", "cache_control": map[string]any{"type": "ephemeral"}}}
	messagesReplayRefresh(t, ctx)
	before := append([]byte(nil), ctx.RawBody...)
	request := messagesReplayFor(cache, r, ctx, selected)
	if err := request.Options().SaveReasoning(messagesReplayFixtureIDs("a"), "hidden"); err != nil {
		t.Fatal(err)
	}
	after, err := json.Marshal(ctx.Body)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("helper mutated the caller's cached request body")
	}
	// Client cache breakpoints may move between turns. They are not transcript
	// semantics, and should not invalidate otherwise identical history.
	ctx.Body["system"] = "Use the Read tool."
	ctx.Body["messages"].([]any)[0].(map[string]any)["content"] = "read"
	messagesReplayAppend(t, ctx, "a")
	next := messagesReplayFor(cache, r, ctx, selected)
	if _, err := messages.ToChatRequest(ctx.RawBody, next.Options()); err != nil || !next.UsedReplay() {
		t.Fatalf("cache hints/text representation broke prefix continuity: %v", err)
	}
}

func TestMessagesReplayToolArgumentNamedCacheControlRemainsSemantic(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	for _, argument := range []string{"A", "B"} {
		r, ctx, selected := messagesReplayFixture(t, "read")
		messagesReplayAppend(t, ctx, "previous")
		assistant := ctx.Body["messages"].([]any)[1].(map[string]any)
		assistant["content"].([]any)[0].(map[string]any)["input"].(map[string]any)["cache_control"] = argument
		messagesReplayRefresh(t, ctx)
		request := messagesReplayFor(cache, r, ctx, selected)
		if err := request.Options().SaveReasoning(messagesReplayFixtureIDs("next"), "reason "+argument); err != nil {
			t.Fatal("semantic tool input was erased from prefix identity")
		}
		messagesReplayAppend(t, ctx, "next")
		options := messagesReplayFor(cache, r, ctx, selected).Options()
		// This callback-only fixture deliberately has no record for the earlier
		// group; it is testing prefix binding, not claiming full conversion.
		if _, found, err := options.LoadReasoning(messagesReplayFixtureIDs("previous")); !errors.Is(err, messages.ErrReasoningReplay) || found {
			t.Fatal("unrecorded owned history did not fail closed")
		}
		value, found, err := options.LoadReasoning(messagesReplayFixtureIDs("next"))
		if err != nil || !found || value != "reason "+argument {
			t.Fatal("tool input change crossed a replay prefix")
		}
	}
	if len(cache.entries) != 2 {
		t.Fatal("two semantic prefixes collapsed into one record")
	}
}

func TestMessagesReplayRawPrecisionAndStaleBody(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	for _, integer := range []string{"9007199254740992", "9007199254740993"} {
		r, ctx, selected := messagesReplayFixture(t, "read")
		messagesReplayAppend(t, ctx, "previous")
		assistant := ctx.Body["messages"].([]any)[1].(map[string]any)
		assistant["content"].([]any)[0].(map[string]any)["input"].(map[string]any)["offset"] = json.Number(integer)
		messagesReplayRefresh(t, ctx)
		// Reproduce PrepareCtx's float64 view while retaining the raw source.
		if err := json.Unmarshal(ctx.RawBody, &ctx.Body); err != nil {
			t.Fatal(err)
		}
		request := messagesReplayFor(cache, r, ctx, selected)
		if err := request.Options().SaveReasoning(messagesReplayFixtureIDs("next"), integer); err != nil {
			t.Fatal("distinct raw integers aliased through the prepared float64 view")
		}
	}
	if len(cache.entries) != 2 {
		t.Fatal("precision-preserving transcript prefixes were not distinct")
	}
	r, ctx, selected := messagesReplayFixture(t, "read")
	ctx.Body["messages"].([]any)[0].(map[string]any)["content"] = "changed without updating RawBody"
	request := messagesReplayFor(cache, r, ctx, selected)
	if err := request.Options().SaveReasoning(messagesReplayFixtureIDs("a"), "hidden"); !errors.Is(err, messages.ErrReasoningReplay) || len(cache.entries) != 2 {
		t.Fatal("stale RawBody was used as the actual input prefix")
	}
}

func TestMessagesReplayDefaultsBoundKnownEmptyRecords(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	for i := 0; i <= messagesReplayMaxEntries; i++ {
		messagesReplaySave(t, cache, "same prompt", "", fmt.Sprintf("call-%d", i))
	}
	if len(cache.entries) != 256 || cache.bytes != 256*messagesReplayEntryCost || cache.bytes > 8<<20 {
		t.Fatal("known-empty records bypassed the default hard count/byte bounds")
	}
	if _, found := messagesReplayLoad(t, cache, "same prompt", "call-0"); found {
		t.Fatal("oldest empty record was not evicted")
	}
	if value, found := messagesReplayLoad(t, cache, "same prompt", "call-256"); !found || value != "" {
		t.Fatal("new empty record is indistinguishable from a miss")
	}
}

func TestMessagesReplayInvalidGroupAndRequestFailClosed(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	r, ctx, selected := messagesReplayFixture(t, "read")
	for _, ids := range [][]string{nil, {""}, {"a", "a"}} {
		request := messagesReplayFor(cache, r, ctx, selected)
		if err := request.Options().SaveReasoning(messagesReplayFixtureIDs(ids...), "hidden"); !errors.Is(err, messages.ErrReasoningReplay) {
			t.Fatal("invalid Save group was accepted")
		}
	}
	for _, bad := range []func(*Ctx){
		func(c *Ctx) { c.Body["messages"] = nil; c.RawBody = nil },
		func(c *Ctx) {
			c.Body["messages"] = []any{map[string]any{"role": "user", "content": 1}}
			c.RawBody = nil
		},
		func(c *Ctx) { c.RawBody = []byte("{") },
		func(c *Ctx) { c.RawBody = []byte(`{"messages":[]}`) },
	} {
		r, ctx, selected := messagesReplayFixture(t, "read")
		bad(ctx)
		request := messagesReplayFor(cache, r, ctx, selected)
		if err := request.Options().SaveReasoning(messagesReplayFixtureIDs("a"), "hidden"); !errors.Is(err, messages.ErrReasoningReplay) {
			t.Fatal("invalid transcript was accepted")
		}
	}
	for _, request := range []*messagesBridgeRequest{nil, newMessagesBridgeRequest(r, nil, selected), newMessagesBridgeRequest(r, ctx, nil)} {
		if err := request.Options().SaveReasoning(messagesReplayFixtureIDs("a"), "hidden"); !errors.Is(err, messages.ErrReasoningReplay) || request.UsedReplay() {
			t.Fatal("unavailable request object reported usable replay")
		}
	}
	if len(cache.entries) != 0 || cache.bytes != 0 {
		t.Fatal("rejected requests retained reasoning")
	}
}

func messagesReplayAppendResponse(t *testing.T, ctx *Ctx, response []byte) []string {
	t.Helper()
	var reply map[string]any
	if err := json.Unmarshal(response, &reply); err != nil {
		t.Fatal(err)
	}
	var ids []string
	var results []any
	for _, value := range reply["content"].([]any) {
		block := value.(map[string]any)
		if block["type"] == "tool_use" {
			id := block["id"].(string)
			ids = append(ids, id)
			results = append(results, map[string]any{"type": "tool_result", "tool_use_id": id, "content": "fixture file text"})
		}
	}
	ctx.Body["messages"] = append(ctx.Body["messages"].([]any),
		map[string]any{"role": "assistant", "content": reply["content"]},
		map[string]any{"role": "user", "content": results})
	messagesReplayRefresh(t, ctx)
	return ids
}

func messagesReplayExpectChannel(t *testing.T, cache *messagesReplayCache, ctx *Ctx, want int64) {
	t.Helper()
	id, required, err := cache.preferredChannel(ctx)
	if err != nil || !required || id == nil || *id != want {
		t.Fatalf("original channel was not required: id=%v, required=%v, error=%v", id, required, err)
	}
}

func messagesReplayExpectRequiredError(t *testing.T, cache *messagesReplayCache, ctx *Ctx) {
	t.Helper()
	id, required, err := cache.preferredChannel(ctx)
	if id != nil || !required || !errors.Is(err, messages.ErrReasoningReplay) {
		t.Fatalf("owned history could escape replay affinity: id=%v, required=%v, error=%v", id, required, err)
	}
	for _, private := range []string{"fixture-downstream-credential", "fixture-upstream-credential", "fixture-session", "private fixture reasoning"} {
		if strings.Contains(err.Error(), private) {
			t.Fatal("affinity error exposed private replay material")
		}
	}
}

func TestMessagesReplayOpaqueToolIDIssuer(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	seen := make(map[string]bool)
	// Independent request objects in the same conversation must not restart a
	// predictable counter, even when the upstream always emits call_0.
	for range 2 {
		r, ctx, selected := messagesReplayFixture(t, "read")
		options := messagesReplayFor(cache, r, ctx, selected).Options()
		for range 128 {
			id, err := options.NewToolUseID()
			if err != nil || !strings.HasPrefix(id, "toolu_mcb_") || len(id) != len("toolu_mcb_")+32 || seen[id] {
				t.Fatal("issuer returned an invalid or repeated opaque ID")
			}
			entropy, err := hex.DecodeString(strings.TrimPrefix(id, "toolu_mcb_"))
			if err != nil || len(entropy) != 16 || strings.ToLower(id) != id {
				t.Fatal("opaque ID is not a 128-bit lowercase hex identity")
			}
			seen[id] = true
		}
	}
	r, ctx, selected := messagesReplayFixture(t, "read")
	ctx.ClientCtx.SessionID = ""
	delete(ctx.Body, "metadata")
	for _, request := range []*messagesBridgeRequest{nil, messagesReplayFor(cache, r, ctx, selected)} {
		if id, err := request.Options().NewToolUseID(); id != "" || !errors.Is(err, messages.ErrReasoningReplay) {
			t.Fatal("issuer marked a source that cannot save replay")
		}
	}
	if len(cache.entries) != 0 {
		t.Fatal("allocating an ID fabricated a completed replay record")
	}
}

func TestMessagesReplayAffinityLeavesNativeHistoryAlone(t *testing.T) {
	t.Parallel()
	// A literal mention in text or tool input is not bridge provenance. Native
	// thinking is also outside this helper's routing decision.
	ctx := &Ctx{Body: map[string]any{"messages": []any{
		map[string]any{"role": "user", "content": "Explain toolu_mcb_0123456789abcdef0123456789abcdef"},
		map[string]any{"role": "assistant", "content": []any{
			map[string]any{"type": "thinking", "thinking": "native-only"},
			map[string]any{"type": "tool_use", "id": "call_0", "name": "Read", "input": map[string]any{"id": "toolu_mcb_0123456789abcdef0123456789abcdef"}},
		}},
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "call_0", "content": "done"}}},
	}}}
	messagesReplayRefresh(t, ctx)
	var absent *messagesReplayCache
	for _, input := range []*Ctx{nil, {}, ctx} {
		for _, preferred := range []func(*Ctx) (*int64, bool, error){messagesBridgePreferredChannel, absent.preferredChannel} {
			if id, required, err := preferred(input); id != nil || required || err != nil {
				t.Fatalf("native history acquired a replay requirement: %v", err)
			}
		}
	}
}

func TestMessagesReplayAffinityClientIsolation(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	messagesReplaySave(t, cache, "read", "private fixture reasoning", "a")
	for _, tc := range []struct {
		name   string
		mutate func(*Ctx)
	}{
		{"downstream credential", func(c *Ctx) { c.Auth.Token = "another-key" }},
		{"downstream key row", func(c *Ctx) { *c.Auth.KeyID++ }},
		{"auth source", func(c *Ctx) { c.Auth.Source = "global" }},
		{"missing auth", func(c *Ctx) { c.Auth = nil }},
		{"profile session", func(c *Ctx) { c.ClientCtx.SessionID = "another-session" }},
		{"opaque session", func(c *Ctx) { c.Body["metadata"].(map[string]any)["user_id"] = "another-opaque-session" }},
		{"missing session", func(c *Ctx) { c.ClientCtx.SessionID = ""; delete(c.Body, "metadata") }},
		{"requested model", func(c *Ctx) { c.RequestedModel = "another-model"; c.Body["model"] = c.RequestedModel }},
		{"missing model", func(c *Ctx) { c.RequestedModel = "" }},
		{"earlier transcript", func(c *Ctx) { c.Body["messages"].([]any)[0].(map[string]any)["content"] = "different prompt" }},
		{"system", func(c *Ctx) { c.Body["system"] = "different system" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, ctx, _ := messagesReplayFixture(t, "read")
			messagesReplayAppend(t, ctx, "a")
			tc.mutate(ctx)
			messagesReplayRefresh(t, ctx)
			messagesReplayExpectRequiredError(t, cache, ctx)
		})
	}
}

func TestMessagesReplayAffinityCompleteOrderedGroup(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	messagesReplaySave(t, cache, "read", "hidden", "a", "b")
	for _, ids := range [][]string{{"a"}, {"b", "a"}, {"a", "b", "c"}, {"a", "a"}} {
		_, ctx, _ := messagesReplayFixture(t, "read")
		messagesReplayAppend(t, ctx, ids...)
		messagesReplayExpectRequiredError(t, cache, ctx)
	}
	_, ctx, selected := messagesReplayFixture(t, "read")
	messagesReplayAppend(t, ctx, "a", "b")
	messagesReplayExpectChannel(t, cache, ctx, selected.Channel.ID)
	// Tool results may arrive in a different order; the source group is the
	// assistant's complete ordered tool_use list, not result arrival order.
	results := ctx.Body["messages"].([]any)[2].(map[string]any)["content"].([]any)
	results[0], results[1] = results[1], results[0]
	messagesReplayRefresh(t, ctx)
	messagesReplayExpectChannel(t, cache, ctx, selected.Channel.ID)
}

func TestMessagesReplayAffinityMalformedProvenance(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	messagesReplaySave(t, cache, "read", "hidden", "a", "b")
	for _, tc := range []struct {
		name   string
		mutate func(*Ctx)
	}{
		{"bad suffix", func(c *Ctx) {
			c.Body["messages"].([]any)[1].(map[string]any)["content"].([]any)[0].(map[string]any)["id"] = "toolu_mcb_bad"
		}},
		{"nonhex suffix", func(c *Ctx) {
			c.Body["messages"].([]any)[1].(map[string]any)["content"].([]any)[0].(map[string]any)["id"] = "toolu_mcb_" + strings.Repeat("z", 32)
		}},
		{"mixed group", func(c *Ctx) {
			c.Body["messages"].([]any)[1].(map[string]any)["content"].([]any)[1].(map[string]any)["id"] = "native_b"
			c.Body["messages"].([]any)[2].(map[string]any)["content"].([]any)[1].(map[string]any)["tool_use_id"] = "native_b"
		}},
		{"incomplete results", func(c *Ctx) {
			m := c.Body["messages"].([]any)[2].(map[string]any)
			m["content"] = m["content"].([]any)[:1]
		}},
		{"orphan result", func(c *Ctx) {
			c.Body["messages"] = append(c.Body["messages"].([]any)[:1], c.Body["messages"].([]any)[2])
		}},
		{"duplicate result", func(c *Ctx) {
			m := c.Body["messages"].([]any)[2].(map[string]any)
			m["content"] = append(m["content"].([]any), m["content"].([]any)[0])
		}},
		{"assistant before complete results", func(c *Ctx) {
			history := c.Body["messages"].([]any)
			c.Body["messages"] = append(history[:2], map[string]any{"role": "assistant", "content": "premature"}, history[2])
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ctx, _ := messagesReplayFixture(t, "read")
			messagesReplayAppend(t, ctx, "a", "b")
			tc.mutate(ctx)
			messagesReplayRefresh(t, ctx)
			messagesReplayExpectRequiredError(t, cache, ctx)
		})
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Ctx)
	}{
		{"only raw retains provenance", func(c *Ctx) { c.Body["messages"] = c.Body["messages"].([]any)[:1] }},
		{"only prepared retains provenance", func(c *Ctx) {
			_, original, _ := messagesReplayFixture(t, "read")
			c.RawBody = original.RawBody
		}},
		{"invalid raw", func(c *Ctx) { c.RawBody = []byte("{") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ctx, _ := messagesReplayFixture(t, "read")
			messagesReplayAppend(t, ctx, "a", "b")
			tc.mutate(ctx) // Deliberately do not re-marshal stale/malformed RawBody.
			messagesReplayExpectRequiredError(t, cache, ctx)
		})
	}
}

func TestMessagesReplayAffinityMissingExpiredOrEvicted(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	now := time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)
	cache.now = func() time.Time { return now }
	messagesReplaySave(t, cache, "read", "", "a")
	_, ctx, selected := messagesReplayFixture(t, "read")
	messagesReplayAppend(t, ctx, "a")
	messagesReplayExpectChannel(t, cache, ctx, selected.Channel.ID)     // Known empty is a source record.
	messagesReplayExpectRequiredError(t, newMessagesReplayCache(), ctx) // Restart / different process.
	messagesReplayExpectRequiredError(t, nil, ctx)
	now = now.Add(messagesReplayTTL - time.Nanosecond)
	messagesReplayExpectChannel(t, cache, ctx, selected.Channel.ID)
	now = now.Add(time.Nanosecond)
	messagesReplayExpectRequiredError(t, cache, ctx) // Affinity reads do not renew TTL.
	if len(cache.entries) != 0 || cache.bytes != 0 {
		t.Fatal("expired affinity retained replay capacity")
	}
	messagesReplaySave(t, cache, "read", "", "a")
	cache.maxEntries = 1
	messagesReplaySave(t, cache, "other", "", "a")
	messagesReplayExpectRequiredError(t, cache, ctx)
}

func TestMessagesReplayAffinityRevalidatesSelectedSource(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		mutate func(*messagesReplayCache, *routing.SelectedChannel)
	}{
		{"channel", func(_ *messagesReplayCache, s *routing.SelectedChannel) { s.Channel.ID++ }},
		{"credential rotation", func(_ *messagesReplayCache, s *routing.SelectedChannel) { s.TokenValue = "rotated-credential" }},
		{"token row", func(_ *messagesReplayCache, s *routing.SelectedChannel) { s.Token.ID++ }},
		{"account", func(_ *messagesReplayCache, s *routing.SelectedChannel) { s.Account.ID++ }},
		{"actual model", func(_ *messagesReplayCache, s *routing.SelectedChannel) { s.ActualModel = "another-upstream-model" }},
		{"evicted between selection and Load", func(c *messagesReplayCache, _ *routing.SelectedChannel) {
			c.maxEntries = 1
			messagesReplaySave(t, c, "other prompt", "", "a")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cache := newMessagesReplayCache()
			messagesReplaySave(t, cache, "read", "", "a")
			r, ctx, selected := messagesReplayFixture(t, "read")
			messagesReplayAppend(t, ctx, "a")
			// Turning off thinking cannot erase previously-owned provenance.
			delete(ctx.Body, "thinking")
			delete(ctx.Body, "output_config")
			messagesReplayRefresh(t, ctx)
			id, required, err := cache.preferredChannel(ctx)
			if err != nil || !required || id == nil || *id != selected.Channel.ID {
				t.Fatal("affinity could not find the original channel")
			}
			tc.mutate(cache, selected)
			next := messagesReplayFor(cache, r, ctx, selected)
			out, err := messages.ToChatRequest(ctx.RawBody, next.Options())
			if !errors.Is(err, messages.ErrReasoningReplay) || len(out) != 0 || next.UsedReplay() || !required {
				t.Fatal("selection hint bypassed full scope revalidation or silently dropped replay")
			}
		})
	}
}

func TestMessagesReplayAffinityPrefixBindsReusedIDs(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	for i, prompt := range []string{"branch A", "branch B"} {
		r, ctx, selected := messagesReplayFixture(t, prompt)
		selected.Channel.ID += int64(i)
		if err := messagesReplayFor(cache, r, ctx, selected).Options().SaveReasoning(messagesReplayFixtureIDs("a"), "hidden"); err != nil {
			t.Fatal(err)
		}
	}
	for i, prompt := range []string{"branch A", "branch B"} {
		_, ctx, selected := messagesReplayFixture(t, prompt)
		messagesReplayAppend(t, ctx, "a")
		messagesReplayExpectChannel(t, cache, ctx, selected.Channel.ID+int64(i))
	}
}

func TestMessagesReplayAffinityRejectsAmbiguousOrMixedSources(t *testing.T) {
	t.Parallel()
	for _, sameGroup := range []bool{true, false} {
		for _, changeChannel := range []bool{true, false} {
			t.Run(fmt.Sprintf("sameGroup=%v/channel=%v", sameGroup, changeChannel), func(t *testing.T) {
				cache := newMessagesReplayCache()
				r, ctx, selected := messagesReplayFixture(t, "read")
				if err := messagesReplayFor(cache, r, ctx, selected).Options().SaveReasoning(messagesReplayFixtureIDs("a"), "hidden"); err != nil {
					t.Fatal(err)
				}
				if !sameGroup {
					messagesReplayAppend(t, ctx, "a")
				}
				if changeChannel {
					selected.Channel.ID++
				} else {
					selected.TokenValue = "rotated-credential"
				}
				id := "a"
				if !sameGroup {
					id = "b"
				}
				if err := messagesReplayFor(cache, r, ctx, selected).Options().SaveReasoning(messagesReplayFixtureIDs(id), "hidden"); err != nil {
					t.Fatal(err)
				}
				messagesReplayAppend(t, ctx, id)
				messagesReplayExpectRequiredError(t, cache, ctx)
			})
		}
	}
	// Finding the latest turn is not enough if an earlier owned group vanished.
	cache := newMessagesReplayCache()
	r, ctx, selected := messagesReplayFixture(t, "read")
	messagesReplayAppend(t, ctx, "missing-earlier")
	if err := messagesReplayFor(cache, r, ctx, selected).Options().SaveReasoning(messagesReplayFixtureIDs("latest"), "hidden"); err != nil {
		t.Fatal(err)
	}
	messagesReplayAppend(t, ctx, "latest")
	messagesReplayExpectRequiredError(t, cache, ctx)
}

func TestMessagesReplayOpaqueIDsAllowRepeatedUpstreamIDsAcrossTurns(t *testing.T) {
	t.Parallel()
	cache := newMessagesReplayCache()
	r, ctx, selected := messagesReplayFixture(t, "Read both files.")
	var issued []string
	for turn := range 3 {
		if turn != 0 {
			messagesReplayExpectChannel(t, cache, ctx, selected.Channel.ID)
		}
		request := messagesReplayFor(cache, r, ctx, selected)
		options := request.Options()
		out, err := messages.ToChatRequest(ctx.RawBody, options)
		if err != nil || request.UsedReplay() != (turn != 0) {
			t.Fatalf("turn %d did not replay its complete history: %v", turn, err)
		}
		var chat map[string]any
		if err := json.Unmarshal(out, &chat); err != nil {
			t.Fatal(err)
		}
		assistant, result := 0, 0
		for _, value := range chat["messages"].([]any) {
			m := value.(map[string]any)
			switch m["role"] {
			case "assistant":
				if m["reasoning_content"] != fmt.Sprintf("private fixture turn %d", assistant) {
					t.Fatal("reasoning replay selected another turn")
				}
				calls := m["tool_calls"].([]any)
				if len(calls) != 2 {
					t.Fatal("parallel tool group was truncated")
				}
				for j, call := range calls {
					if call.(map[string]any)["id"] != issued[2*assistant+j] {
						t.Fatal("assistant ID was reverted to the reused upstream ID")
					}
				}
				assistant++
			case "tool":
				if m["tool_call_id"] != issued[result] {
					t.Fatal("tool result lost its caller-owned ID")
				}
				result++
			}
		}
		if assistant != turn || result != 2*turn {
			t.Fatal("continuation dropped historical turns")
		}
		reply, err := messages.FromChatResponse(messagesReplayResponse(t, fmt.Sprintf("private fixture turn %d", turn), "call_0", "call_1"), options)
		if err != nil || bytes.Contains(reply, []byte("private fixture")) || bytes.Contains(reply, []byte("reasoning_content")) {
			t.Fatalf("tool response failed or exposed hidden reasoning: %v", err)
		}
		ids := messagesReplayAppendResponse(t, ctx, reply)
		if len(ids) != 2 {
			t.Fatal("native response lost a parallel tool")
		}
		for _, id := range ids {
			if !strings.HasPrefix(id, "toolu_mcb_") {
				t.Fatal("tool response lacked owned provenance")
			}
			for _, previous := range issued {
				if id == previous {
					t.Fatal("repeated upstream ID became a repeated caller ID")
				}
			}
			issued = append(issued, id)
		}
	}
	messagesReplayExpectChannel(t, cache, ctx, selected.Channel.ID)
}
