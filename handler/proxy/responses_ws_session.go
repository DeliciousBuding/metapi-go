package proxyhandler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
)

type responsesWSSession struct {
	conn      *websocket.Conn
	req       *http.Request
	auth      *auth.ProxyAuthContext
	path      string
	turnState string
	// clientIP is captured at handshake time so each message can re-run the
	// key's IP rules without a per-message HTTP request.
	clientIP string

	mu          sync.Mutex
	lastRequest map[string]any
	lastOutput  []any
	// Client-facing WS session id for Codex upstream runtime keying (process-local).
	clientSessionID string
	// Upstream Codex wss session keys opened this connection (closed on defer).
	codexRuntimeKeys     []string
	directRuntimeKey     string
	directAuthentication *channelplugins.AuthenticationLease
	// message serialisation: one in-flight turn at a time (Codex client queue).
	queueMu sync.Mutex
}

func (s *responsesWSSession) loop(ctx context.Context) error {
	ctx, cancelSession := context.WithCancel(ctx)
	defer cancelSession()
	// Canceling a pending Read closes its socket without a close handshake.
	// Finish the peer handshake first, then stop the session reader.
	defer func() { _ = s.conn.Close(websocket.StatusNormalClosure, "session end") }()
	type incoming struct {
		data []byte
		err  error
	}
	// Keep reading while a turn is upstream so downstream disconnect cancels
	// its queue wait/socket read immediately. Only one pending frame is held;
	// all authorization and turn execution remains serial below.
	messages := make(chan incoming, 1)
	go func() {
		for {
			readCtx, cancel := context.WithTimeout(ctx, wsReadIdleTimeout)
			kind, data, err := s.conn.Read(readCtx)
			cancel()
			if err != nil {
				cancelSession()
				select {
				case messages <- incoming{err: err}:
				default:
				}
				return
			}
			if kind != websocket.MessageText && kind != websocket.MessageBinary {
				continue
			}
			select {
			case messages <- incoming{data: data}:
			case <-ctx.Done():
				return
			default:
				// A caller may queue one following turn. Do not retain an
				// unbounded client burst while an upstream turn is running.
				cancelSession()
				_ = s.conn.CloseNow()
				return
			}
		}
	}()
	for {
		var next incoming
		select {
		case next = <-messages:
		case <-ctx.Done():
			return ctx.Err()
		}
		if next.err != nil {
			return next.err
		}
		s.queueMu.Lock()
		err := s.handleMessage(ctx, next.data)
		s.queueMu.Unlock()
		if err != nil {
			if errors.Is(err, errResponsesWSSessionEnded) {
				return err
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Debug("responses websocket turn error", "err", err)
		}
	}
}
