package proxyhandler

import (
	"container/list"
	"crypto/sha256"
	"encoding/json"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
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
	if channelID == 0 {
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
		if matched || entry.channelID == 0 {
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
