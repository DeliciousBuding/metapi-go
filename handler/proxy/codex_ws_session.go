package proxyhandler

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/config"
)

const (
	codexSessionTTLMin  = 5 * time.Minute
	codexSessionMaxKeys = 10_000
)

type sessionResponseEntry struct {
	responseID  string
	updatedAtMs int64
}

var (
	codexSessionResponseMu  sync.Mutex
	codexSessionResponseIDs = map[string]sessionResponseEntry{}
)

func codexSessionTTL() time.Duration {
	ttlMs := config.DefaultProxyStickySessionTtlMs
	if cfg := safeConfigGet(); cfg != nil && cfg.ProxyStickySessionTtlMs > 0 {
		ttlMs = cfg.ProxyStickySessionTtlMs
	}
	d := time.Duration(ttlMs) * time.Millisecond
	if d < codexSessionTTLMin {
		return codexSessionTTLMin
	}
	return d
}

func safeConfigGet() *config.Config {
	defer func() { _ = recover() }()
	return config.Get()
}

func normalizeCodexSessionKey(sessionID string) string {
	return strings.TrimSpace(sessionID)
}

// BuildCodexSessionResponseStoreKey scopes a WS client session to site/account/channel.
// Empty sessionID → empty key.
func BuildCodexSessionResponseStoreKey(sessionID string, siteID, accountID, channelID int64) string {
	sid := normalizeCodexSessionKey(sessionID)
	if sid == "" {
		return ""
	}
	parts := make([]string, 0, 4)
	if siteID > 0 {
		parts = append(parts, fmt.Sprintf("site:%d", siteID))
	}
	if accountID > 0 {
		parts = append(parts, fmt.Sprintf("account:%d", accountID))
	}
	if channelID > 0 {
		parts = append(parts, fmt.Sprintf("channel:%d", channelID))
	}
	parts = append(parts, "session:"+sid)
	return strings.Join(parts, "|")
}

func getCodexSessionResponseID(sessionID string) string {
	key := normalizeCodexSessionKey(sessionID)
	if key == "" {
		return ""
	}
	now := time.Now().UnixMilli()
	ttl := codexSessionTTL().Milliseconds()
	codexSessionResponseMu.Lock()
	defer codexSessionResponseMu.Unlock()
	sweepExpiredSessionResponseIDsLocked(now, ttl)
	entry, ok := codexSessionResponseIDs[key]
	if !ok {
		return ""
	}
	if entry.updatedAtMs+ttl <= now {
		delete(codexSessionResponseIDs, key)
		return ""
	}
	entry.updatedAtMs = now
	codexSessionResponseIDs[key] = entry
	return entry.responseID
}

func setCodexSessionResponseID(sessionID, responseID string) {
	key := normalizeCodexSessionKey(sessionID)
	rid := strings.TrimSpace(responseID)
	if key == "" || rid == "" {
		return
	}
	now := time.Now().UnixMilli()
	ttl := codexSessionTTL().Milliseconds()
	codexSessionResponseMu.Lock()
	defer codexSessionResponseMu.Unlock()
	sweepExpiredSessionResponseIDsLocked(now, ttl)
	if len(codexSessionResponseIDs) >= codexSessionMaxKeys {
		// Drop oldest entry (linear scan; bounded by max keys).
		var oldestKey string
		var oldestMs int64
		for k, e := range codexSessionResponseIDs {
			if oldestKey == "" || e.updatedAtMs < oldestMs {
				oldestKey = k
				oldestMs = e.updatedAtMs
			}
		}
		if oldestKey != "" {
			delete(codexSessionResponseIDs, oldestKey)
		}
	}
	codexSessionResponseIDs[key] = sessionResponseEntry{responseID: rid, updatedAtMs: now}
}

func clearCodexSessionResponseID(sessionID string) {
	key := normalizeCodexSessionKey(sessionID)
	if key == "" {
		return
	}
	codexSessionResponseMu.Lock()
	delete(codexSessionResponseIDs, key)
	codexSessionResponseMu.Unlock()
}

func sweepExpiredSessionResponseIDsLocked(nowMs, ttlMs int64) {
	for k, e := range codexSessionResponseIDs {
		if e.updatedAtMs+ttlMs <= nowMs {
			delete(codexSessionResponseIDs, k)
		}
	}
}

// ResetCodexSessionResponseStoreForTest clears process-local state (tests only).
func ResetCodexSessionResponseStoreForTest() {
	codexSessionResponseMu.Lock()
	codexSessionResponseIDs = map[string]sessionResponseEntry{}
	codexSessionResponseMu.Unlock()
}

// ---- socket session store ----

type codexWSSocketSession struct {
	sessionID string
	conn      *websocket.Conn
	socketURL string
	mu        sync.Mutex // serialise turns on one session
	lastUsed  time.Time
}

var (
	codexWSSocketMu       sync.Mutex
	codexWSSocketSessions = map[string]*codexWSSocketSession{}
)

func getOrCreateCodexWSSocketSession(sessionID string) *codexWSSocketSession {
	key := normalizeCodexSessionKey(sessionID)
	codexWSSocketMu.Lock()
	defer codexWSSocketMu.Unlock()
	if s, ok := codexWSSocketSessions[key]; ok {
		return s
	}
	if len(codexWSSocketSessions) >= codexSessionMaxKeys {
		return nil
	}
	s := &codexWSSocketSession{sessionID: key}
	codexWSSocketSessions[key] = s
	return s
}

func takeCodexWSSocketSession(sessionID string) *codexWSSocketSession {
	key := normalizeCodexSessionKey(sessionID)
	codexWSSocketMu.Lock()
	defer codexWSSocketMu.Unlock()
	s := codexWSSocketSessions[key]
	delete(codexWSSocketSessions, key)
	return s
}

func clearSessionSocket(session *codexWSSocketSession, conn *websocket.Conn) {
	if session == nil {
		return
	}
	if session.conn == conn {
		session.conn = nil
		session.socketURL = ""
	}
}

// CloseCodexWebsocketSession closes the upstream socket for a session key.
// Intentionally preserves remembered previous_response_id (TS parity).
func CloseCodexWebsocketSession(sessionID string) {
	s := takeCodexWSSocketSession(sessionID)
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn != nil {
		_ = s.conn.Close(websocket.StatusNormalClosure, "session close")
		s.conn = nil
		s.socketURL = ""
	}
}
