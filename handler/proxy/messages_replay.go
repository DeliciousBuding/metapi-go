package proxyhandler

import (
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

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

func messagesReplayUnavailable(reason string) error {
	// reason is always a static diagnostic, never a credential, ID, transcript,
	// reasoning value, or a wrapped storage/parser error containing those values.
	return fmt.Errorf("%w: %s", messages.ErrReasoningReplay, reason)
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
	if selected != nil && selected.Direct != nil {
		direct := selected.Direct
		if direct.RouteID <= 0 || direct.ItemID <= 0 || direct.ChannelID <= 0 || direct.CredentialID <= 0 || direct.ModelID <= 0 || direct.GrantID <= 0 || selected.Channel.ID != -direct.ItemID || strings.TrimSpace(selected.ActualModel) == "" || strings.TrimSpace(selected.TokenValue) == "" && !direct.HasAnonymousCredential() {
			return [sha256.Size]byte{}, messagesReplayUnavailable("selected direct upstream identity is unavailable")
		}
		endpoints, err := json.Marshal(direct.Endpoints)
		if err != nil {
			return [sha256.Size]byte{}, messagesReplayUnavailable("selected direct endpoint identity is invalid")
		}
		order, _ := json.Marshal(direct.ProtocolOrder)
		requestConfig, err := json.Marshal(direct.RequestConfig)
		if err != nil {
			return [sha256.Size]byte{}, messagesReplayUnavailable("selected request configuration is invalid")
		}
		return messagesReplayHash("messages-replay-direct-v1", string(client[:]), selected.ActualModel,
			strconv.FormatInt(direct.RouteID, 10), strconv.FormatInt(direct.GroupID, 10), strconv.FormatInt(direct.ItemID, 10), strconv.FormatInt(direct.ChannelID, 10), strconv.FormatInt(direct.CredentialID, 10), strconv.FormatInt(direct.GrantID, 10), strconv.FormatInt(direct.ModelID, 10),
			selected.TokenValue, direct.CredentialKind, direct.Provider, direct.BaseURL, direct.ChatPath, direct.ResponsesPath, direct.AnthropicPath,
			direct.CustomHeader, direct.ParamOverride, string(requestConfig), string(endpoints), strconv.Itoa(direct.Protocols), string(order)), nil
	}
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
