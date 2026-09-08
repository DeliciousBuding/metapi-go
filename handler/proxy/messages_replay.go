package proxyhandler

import (
	"bytes"
	"container/list"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

const (
	messagesReplayMaxEntries = 256
	messagesReplayMaxBytes   = 8 << 20
	messagesReplayMaxRecord  = 512 << 10
	messagesReplayTTL        = 30 * time.Minute
	// Charge fixed key/entry overhead as well as payload, including empty
	// reasoning records. This bounds retained data, not Go allocator RSS.
	messagesReplayEntryCost    = 512
	messagesBridgeToolIDPrefix = "toolu_mcb_"
)

// Only the caller layer owns cross-request state. It is process-local and
// best-effort: no logs, disk, per-record timers, or cross-process affinity.
var messagesBridgeReplayCache = newMessagesReplayCache()

type messagesReplayKey struct {
	client, scope, prefix, group [sha256.Size]byte
}

type messagesReplayEntry struct {
	key       messagesReplayKey
	reasoning string
	channelID int64
	expires   time.Time
	cost      int
}

type messagesReplayCache struct {
	mu                                   sync.Mutex
	entries                              map[messagesReplayKey]*list.Element
	lru                                  list.List
	bytes                                int
	maxEntries, maxBytes, maxRecordBytes int
	ttl                                  time.Duration
	now                                  func() time.Time
}

func newMessagesReplayCache() *messagesReplayCache {
	return &messagesReplayCache{
		entries:    make(map[messagesReplayKey]*list.Element),
		maxEntries: messagesReplayMaxEntries, maxBytes: messagesReplayMaxBytes,
		maxRecordBytes: messagesReplayMaxRecord, ttl: messagesReplayTTL, now: time.Now,
	}
}

func messagesReplayUnavailable(reason string) error {
	// reason is always a static diagnostic, never a credential, ID, transcript,
	// reasoning value, or a wrapped storage/parser error containing those values.
	return fmt.Errorf("%w: %s", messages.ErrReasoningReplay, reason)
}

func (c *messagesReplayCache) remove(element *list.Element) {
	entry := element.Value.(*messagesReplayEntry)
	delete(c.entries, entry.key)
	c.bytes -= entry.cost
	c.lru.Remove(element)
}

func (c *messagesReplayCache) expire(now time.Time) {
	for _, element := range c.entries {
		if !now.Before(element.Value.(*messagesReplayEntry).expires) {
			c.remove(element)
		}
	}
}

func (c *messagesReplayCache) put(key messagesReplayKey, channelID int64, reasoning string) error {
	if channelID <= 0 {
		return messagesReplayUnavailable("source channel is unavailable")
	}
	if !utf8.ValidString(reasoning) {
		return messagesReplayUnavailable("invalid reasoning encoding")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.maxEntries < 1 || c.ttl <= 0 || len(reasoning) > c.maxRecordBytes || c.maxBytes < messagesReplayEntryCost || len(reasoning) > c.maxBytes-messagesReplayEntryCost {
		return messagesReplayUnavailable("reasoning exceeds replay capacity")
	}
	now := c.now()
	c.expire(now)
	if existing := c.entries[key]; existing != nil {
		entry := existing.Value.(*messagesReplayEntry)
		if entry.reasoning != reasoning || entry.channelID != channelID {
			// An identical prefix/group must not silently change its reasoning.
			// The new terminal fails; the earlier valid record remains intact.
			return messagesReplayUnavailable("conflicting replay record")
		}
		entry.expires = now.Add(c.ttl)
		c.lru.MoveToFront(existing)
		return nil
	}
	cost := len(reasoning) + messagesReplayEntryCost
	for len(c.entries) >= c.maxEntries || c.bytes > c.maxBytes-cost {
		c.remove(c.lru.Back())
	}
	entry := &messagesReplayEntry{key: key, channelID: channelID, reasoning: strings.Clone(reasoning), expires: now.Add(c.ttl), cost: cost}
	c.entries[key] = c.lru.PushFront(entry)
	c.bytes += cost
	return nil
}

func (c *messagesReplayCache) get(key messagesReplayKey) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expire(c.now())
	element := c.entries[key]
	if element == nil {
		return "", false
	}
	c.lru.MoveToFront(element)
	// Reads affect eviction order, not absolute expiry.
	return element.Value.(*messagesReplayEntry).reasoning, true
}

type messagesReplayHistory struct {
	group, prefix [sha256.Size]byte
	owned         bool
}

// messagesBridgeRequest snapshots one selected upstream attempt. Scope and
// transcript data are reduced to hashes; only completed reasoning is cached.
type messagesBridgeRequest struct {
	cache                 *messagesReplayCache
	requestCtx            context.Context
	client, scope, prefix [sha256.Size]byte
	channelID             int64
	history               []messagesReplayHistory
	used                  atomic.Bool
	err                   error
}

func newMessagesBridgeRequest(r *http.Request, ctx *Ctx, selected *routing.SelectedChannel) *messagesBridgeRequest {
	request := &messagesBridgeRequest{cache: messagesBridgeReplayCache, requestCtx: context.Background()}
	if r != nil {
		request.requestCtx = r.Context()
	}
	request.client, request.err = messagesReplayClientScope(ctx)
	if request.err == nil {
		request.scope, request.err = messagesReplayScope(ctx, selected, request.client)
	}
	if request.err == nil {
		request.channelID = selected.Channel.ID
		request.prefix, request.history, request.err = messagesReplayPrefixes(ctx)
	}
	return request
}

func (b *messagesBridgeRequest) ready() error {
	if b == nil || b.cache == nil {
		return messagesReplayUnavailable("replay request is unavailable")
	}
	if b.err != nil {
		return b.err
	}
	if b.requestCtx.Err() != nil {
		return messagesReplayUnavailable("request is canceled")
	}
	return nil
}

// Options returns callbacks for one ToChatRequest conversion and its response.
// Loads consume assistant tool groups in transcript order, so repeated IDs in
// different historical turns cannot select each other's prefix. Obtain fresh
// Options for a new conversion/retry; UsedReplay remains sticky for this object.
func (b *messagesBridgeRequest) Options() messages.Options {
	var mu sync.Mutex
	next := 0
	return messages.Options{
		NewToolUseID: func() (string, error) {
			if err := b.ready(); err != nil {
				return "", err
			}
			var entropy [16]byte
			if _, err := rand.Read(entropy[:]); err != nil {
				return "", messagesReplayUnavailable("tool identity allocation failed")
			}
			return messagesBridgeToolIDPrefix + hex.EncodeToString(entropy[:]), nil
		},
		LoadReasoning: func(ids []string) (string, bool, error) {
			mu.Lock()
			defer mu.Unlock()
			if err := b.ready(); err != nil {
				return "", false, err
			}
			group, err := messagesReplayGroup(ids)
			if err != nil {
				return "", false, err
			}
			if next >= len(b.history) || b.history[next].group != group {
				return "", false, messagesReplayUnavailable("tool group does not match request history order")
			}
			history := b.history[next]
			next++
			value, found := b.cache.get(messagesReplayKey{client: b.client, scope: b.scope, prefix: history.prefix, group: group})
			if found {
				b.used.Store(true)
			} else if history.owned {
				return "", false, messagesReplayUnavailable("bridge-owned tool history has no live replay record")
			}
			// Missing/expired/evicted and known-empty are intentionally different.
			// ToChatRequest fails closed on an adaptive history cache miss.
			return value, found, nil
		},
		SaveReasoning: func(ids []string, reasoning string) error {
			if err := b.ready(); err != nil {
				return err
			}
			group, err := messagesReplayGroup(ids)
			if err != nil {
				return err
			}
			owned, err := messagesReplayOwnedGroup(ids)
			if err != nil {
				return err
			}
			if !owned {
				return messagesReplayUnavailable("tool IDs lack bridge provenance")
			}
			return b.cache.put(messagesReplayKey{client: b.client, scope: b.scope, prefix: b.prefix, group: group}, b.channelID, reasoning)
		},
	}
}

// UsedReplay is true after any successful Load, including known-empty reasoning.
// Check it after a successful request conversion before choosing the endpoint:
// replayed Chat history must not first be sent as lossy native Messages.
func (b *messagesBridgeRequest) UsedReplay() bool {
	return b != nil && b.used.Load()
}

func messagesReplayHash(parts ...string) [sha256.Size]byte {
	h := sha256.New()
	var length [8]byte
	for _, part := range parts {
		binary.BigEndian.PutUint64(length[:], uint64(len(part)))
		h.Write(length[:])
		h.Write([]byte(part))
	}
	var result [sha256.Size]byte
	copy(result[:], h.Sum(nil))
	return result
}

func messagesReplayGroup(ids []string) ([sha256.Size]byte, error) {
	seen := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return [sha256.Size]byte{}, messagesReplayUnavailable("tool group is empty")
	}
	for _, id := range ids {
		if strings.TrimSpace(id) == "" || seen[id] {
			return [sha256.Size]byte{}, messagesReplayUnavailable("tool group has invalid or duplicate IDs")
		}
		seen[id] = true
	}
	return messagesReplayHash(ids...), nil
}

func messagesReplayClientScope(ctx *Ctx) ([sha256.Size]byte, error) {
	unavailable := func(reason string) ([sha256.Size]byte, error) {
		return [sha256.Size]byte{}, messagesReplayUnavailable(reason)
	}
	if ctx == nil || ctx.Auth == nil || (ctx.Auth.Source != "managed" && ctx.Auth.Source != "global") {
		return unavailable("authenticated principal is unavailable")
	}
	keyID := int64(0)
	if ctx.Auth.KeyID != nil {
		keyID = *ctx.Auth.KeyID
	}
	if strings.TrimSpace(ctx.Auth.Token) == "" && (ctx.Auth.Source != "managed" || keyID <= 0) {
		return unavailable("authenticated key identity is unavailable")
	}
	meta, _ := ctx.Body["metadata"].(map[string]any)
	userID, _ := meta["user_id"].(string)
	session := strings.TrimSpace(ctx.ClientCtx.SessionID)
	if session == "" && strings.TrimSpace(userID) == "" {
		return unavailable("stable conversation identity is unavailable")
	}
	if strings.TrimSpace(ctx.RequestedModel) == "" {
		return unavailable("requested model is unavailable")
	}
	// Metadata is a conversation discriminator, never authentication. Raw keys
	// and session values are reduced to a hash, not retained in cache entries.
	return messagesReplayHash("messages-replay-client-v1", ctx.Auth.Source,
		strconv.FormatInt(keyID, 10), ctx.Auth.Token, session, userID, ctx.RequestedModel), nil
}

func messagesReplayScope(ctx *Ctx, selected *routing.SelectedChannel, client [sha256.Size]byte) ([sha256.Size]byte, error) {
	if selected == nil || selected.Channel.ID <= 0 || (selected.Site.ID <= 0 && strings.TrimSpace(selected.Site.URL) == "") || (selected.Account.ID <= 0 && strings.TrimSpace(selected.TokenValue) == "") {
		return [sha256.Size]byte{}, messagesReplayUnavailable("selected upstream identity is unavailable")
	}
	actualModel := selected.ActualModel
	if actualModel == "" {
		actualModel = ctx.RequestedModel
	}
	tokenID := int64(0)
	if selected.Token != nil {
		tokenID = selected.Token.ID
	} else if selected.Channel.TokenID != nil {
		tokenID = *selected.Channel.TokenID
	}
	return messagesReplayHash("messages-replay-upstream-v2", string(client[:]), actualModel,
		strconv.FormatInt(selected.Site.ID, 10), selected.Site.URL, selected.Site.Platform,
		strconv.FormatInt(selected.Account.ID, 10), strconv.FormatInt(selected.Channel.ID, 10),
		strconv.FormatInt(tokenID, 10), selected.TokenValue), nil
}

// Decode a snapshot, never mutate Ctx.Body. Prefer precision-preserving RawBody
// messages when they agree with the prepared body; fail rather than key stale
// raw input after a caller has changed the transcript.
func messagesReplayInput(ctx *Ctx) ([]any, error) {
	encoded, err := json.Marshal(ctx.Body["messages"])
	if err != nil {
		return nil, messagesReplayUnavailable("invalid request transcript")
	}
	if len(ctx.RawBody) != 0 {
		var raw map[string]json.RawMessage
		if json.Unmarshal(ctx.RawBody, &raw) != nil || len(raw["messages"]) == 0 {
			return nil, messagesReplayUnavailable("invalid raw request transcript")
		}
		var precise any
		decoder := json.NewDecoder(bytes.NewReader(raw["messages"]))
		decoder.UseNumber()
		if decoder.Decode(&precise) != nil {
			return nil, messagesReplayUnavailable("invalid raw request transcript")
		}
		prepared, err := json.Marshal(precise)
		if err != nil {
			return nil, messagesReplayUnavailable("invalid raw request transcript")
		}
		if !bytes.Equal(prepared, encoded) {
			// PrepareCtx currently decodes numbers as float64. Compare that view
			// too, but hash the exact raw numbers so large integers do not alias.
			var view any
			if json.Unmarshal(raw["messages"], &view) != nil {
				return nil, messagesReplayUnavailable("invalid raw request transcript")
			}
			prepared, err = json.Marshal(view)
			if err != nil || !bytes.Equal(prepared, encoded) {
				return nil, messagesReplayUnavailable("raw and prepared transcripts disagree")
			}
		}
		encoded = raw["messages"]
	}
	var history []any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if decoder.Decode(&history) != nil || len(history) == 0 {
		return nil, messagesReplayUnavailable("request transcript is empty or invalid")
	}
	return history, nil
}

// Ignore only protocol-level cache hints, not similarly named tool arguments.
// A string and a single text block are the same transcript content. Other
// changes (including compaction) deliberately change the prefix and miss cache.
func messagesReplayContent(value any) (any, error) {
	if text, ok := value.(string); ok {
		return []any{map[string]any{"type": "text", "text": text}}, nil
	}
	blocks, ok := value.([]any)
	if !ok {
		return nil, messagesReplayUnavailable("invalid transcript content")
	}
	out := make([]any, 0, len(blocks))
	for _, value := range blocks {
		block, ok := value.(map[string]any)
		if !ok {
			return nil, messagesReplayUnavailable("invalid transcript content block")
		}
		copy := make(map[string]any, len(block))
		for key, value := range block {
			if key != "cache_control" {
				copy[key] = value
			}
		}
		if copy["type"] == "tool_result" && copy["content"] != nil {
			content, err := messagesReplayContent(copy["content"])
			if err != nil {
				return nil, err
			}
			copy["content"] = content
		}
		out = append(out, copy)
	}
	return out, nil
}

func messagesReplayPrefixes(ctx *Ctx) ([sha256.Size]byte, []messagesReplayHistory, error) {
	history, err := messagesReplayInput(ctx)
	if err != nil {
		return [sha256.Size]byte{}, nil, err
	}
	var system []byte
	if ctx.Body["system"] != nil {
		value, err := messagesReplayContent(ctx.Body["system"])
		if err != nil {
			return [sha256.Size]byte{}, nil, err
		}
		system, err = json.Marshal(value)
		if err != nil {
			return [sha256.Size]byte{}, nil, messagesReplayUnavailable("invalid system content")
		}
	}
	prefix := messagesReplayHash("messages-replay-prefix-v1", string(system))
	var groups []messagesReplayHistory
	pendingOwned := make(map[string]bool)
	for _, value := range history {
		message, ok := value.(map[string]any)
		if !ok || (message["role"] != "user" && message["role"] != "assistant") {
			return [sha256.Size]byte{}, nil, messagesReplayUnavailable("invalid transcript message")
		}
		content, err := messagesReplayContent(message["content"])
		if err != nil {
			return [sha256.Size]byte{}, nil, err
		}
		message["content"] = content
		if message["role"] == "assistant" {
			if len(pendingOwned) != 0 {
				return [sha256.Size]byte{}, nil, messagesReplayUnavailable("bridge-owned tools were not fully answered")
			}
			var ids []string
			for _, value := range content.([]any) {
				block := value.(map[string]any)
				if block["type"] == "tool_use" {
					id, _ := block["id"].(string)
					ids = append(ids, id)
				}
			}
			if len(ids) != 0 {
				group, err := messagesReplayGroup(ids)
				if err != nil {
					return [sha256.Size]byte{}, nil, err
				}
				owned, err := messagesReplayOwnedGroup(ids)
				if err != nil {
					return [sha256.Size]byte{}, nil, err
				}
				groups = append(groups, messagesReplayHistory{group: group, prefix: prefix, owned: owned})
				if owned {
					for _, id := range ids {
						pendingOwned[id] = true
					}
				}
			}
		}
		if message["role"] == "user" {
			for _, value := range content.([]any) {
				block := value.(map[string]any)
				id, _ := block["tool_use_id"].(string)
				if block["type"] == "tool_result" && strings.HasPrefix(id, messagesBridgeToolIDPrefix) {
					if !pendingOwned[id] {
						return [sha256.Size]byte{}, nil, messagesReplayUnavailable("bridge-owned tool result has no matching call")
					}
					delete(pendingOwned, id)
				}
			}
		}
		encoded, err := json.Marshal(message)
		if err != nil {
			return [sha256.Size]byte{}, nil, messagesReplayUnavailable("invalid transcript message")
		}
		// A hash chain is linear in transcript bytes, not an O(n^2) serialization
		// of every prefix. No transcript strings remain in the replay cache.
		prefix = messagesReplayHash(string(prefix[:]), string(encoded))
	}
	if len(pendingOwned) != 0 {
		return [sha256.Size]byte{}, nil, messagesReplayUnavailable("bridge-owned tools were not fully answered")
	}
	return prefix, groups, nil
}

// The prefix is a provenance marker, not authorization or a persistence token.
// Any malformed/mixed use of the reserved namespace remains fail-closed.
func messagesReplayOwnedGroup(ids []string) (bool, error) {
	owned, ordinary := false, false
	for _, id := range ids {
		if !strings.HasPrefix(id, messagesBridgeToolIDPrefix) {
			ordinary = true
			continue
		}
		owned = true
		suffix := strings.TrimPrefix(id, messagesBridgeToolIDPrefix)
		if len(suffix) != 32 {
			return false, messagesReplayUnavailable("malformed bridge-owned tool ID")
		}
		decoded, err := hex.DecodeString(suffix)
		if err != nil || len(decoded) != 16 {
			return false, messagesReplayUnavailable("malformed bridge-owned tool ID")
		}
	}
	if owned && ordinary {
		return false, messagesReplayUnavailable("mixed bridge-owned and native tool group")
	}
	return owned, nil
}

func messagesReplayHasOwnedIDs(value any) bool {
	history, _ := value.([]any)
	for _, item := range history {
		message, _ := item.(map[string]any)
		content, _ := message["content"].([]any)
		for _, item := range content {
			block, _ := item.(map[string]any)
			for _, field := range []string{"id", "tool_use_id"} {
				id, _ := block[field].(string)
				if strings.HasPrefix(id, messagesBridgeToolIDPrefix) {
					return true
				}
			}
		}
	}
	return false
}

// messagesBridgePreferredChannel must run before ordinary channel selection.
// required stays true on every error involving our tool namespace; callers must
// retain it across policy rejection, disabled fallback, conversion failure and
// a later Load miss. Never route such history to bare native Messages.
//
// The returned ID is only a routing hint. SelectPreferredChannel must still
// enforce the current downstream policy and channel availability. Options.Load
// subsequently checks the full selected upstream/credential/model identity.
func messagesBridgePreferredChannel(ctx *Ctx) (channelID *int64, required bool, err error) {
	return messagesBridgeReplayCache.preferredChannel(ctx)
}

func (c *messagesReplayCache) preferredChannel(ctx *Ctx) (*int64, bool, error) {
	if ctx == nil {
		return nil, false, nil
	}
	required := messagesReplayHasOwnedIDs(ctx.Body["messages"])
	if len(ctx.RawBody) != 0 {
		var raw map[string]json.RawMessage
		if json.Unmarshal(ctx.RawBody, &raw) == nil {
			var history []any
			if json.Unmarshal(raw["messages"], &history) == nil {
				required = required || messagesReplayHasOwnedIDs(history)
			}
		}
	}
	if !required {
		// Do not impose replay authentication/session requirements on ordinary
		// native tools, including native features the Chat bridge cannot encode.
		return nil, false, nil
	}
	fail := func(reason string) (*int64, bool, error) {
		return nil, true, messagesReplayUnavailable(reason)
	}
	if c == nil {
		return fail("replay cache is unavailable")
	}
	client, err := messagesReplayClientScope(ctx)
	if err != nil {
		return nil, true, err
	}
	_, history, err := messagesReplayPrefixes(ctx)
	if err != nil {
		return nil, true, err
	}
	type lookupKey struct{ prefix, group [sha256.Size]byte }
	needed := make(map[lookupKey]bool)
	for _, group := range history {
		if !group.owned {
			continue
		}
		key := lookupKey{prefix: group.prefix, group: group.group}
		if _, duplicate := needed[key]; duplicate {
			return fail("ambiguous bridge-owned history")
		}
		needed[key] = false
	}
	if len(needed) == 0 {
		return fail("bridge-owned history has no complete tool group")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(needed) > c.maxEntries {
		return fail("bridge-owned history exceeds replay capacity")
	}
	c.expire(c.now())
	var channelID int64
	var origin [sha256.Size]byte
	found := 0
	// One bounded scan, not a second reverse-index cache with its own lifecycle.
	for _, element := range c.entries {
		entry := element.Value.(*messagesReplayEntry)
		if entry.key.client != client {
			continue
		}
		key := lookupKey{prefix: entry.key.prefix, group: entry.key.group}
		matched, wanted := needed[key]
		if !wanted {
			continue
		}
		if matched || entry.channelID <= 0 {
			return fail("ambiguous bridge-owned replay source")
		}
		if found != 0 && (channelID != entry.channelID || origin != entry.key.scope) {
			return fail("bridge-owned groups do not share one upstream source")
		}
		needed[key] = true
		channelID, origin = entry.channelID, entry.key.scope
		found++
	}
	if found != len(needed) {
		return fail("bridge-owned replay source is missing or expired")
	}
	return &channelID, true, nil
}
