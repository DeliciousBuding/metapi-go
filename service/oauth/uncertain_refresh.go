package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptrace"
	"sync/atomic"

	"github.com/deliciousbuding/metapi-go/store"
)

var ErrOAuthRefreshUncertain = errors.New("OAuth refresh result uncertain; reauthorize or replace the credential")

var errRefreshNotConsumed = errors.New("OAuth refresh token was not consumed")

type refreshExchangeIdentityKey struct{}
type refreshExchangeIdentity struct{ AccessToken, IssuerScope, Fingerprint string }

func (refreshExchangeIdentity) String() string { return "refreshExchangeIdentity([REDACTED])" }

type refreshNotConsumedError struct{ cause error }

func (e *refreshNotConsumedError) Error() string        { return e.cause.Error() }
func (e *refreshNotConsumedError) Unwrap() error        { return e.cause }
func (e *refreshNotConsumedError) Is(target error) bool { return target == errRefreshNotConsumed }

func refreshNotConsumed(err error) error {
	return &refreshNotConsumedError{cause: err}
}

// Built-in token endpoints use doHTTP's standard transport. A connected
// failure is conservatively uncertain even without a complete response.
func doRefreshHTTP(req *http.Request, proxyURL *string, refresh bool) (*http.Response, error) {
	if !refresh {
		return doHTTP(req, proxyURL, nil)
	}
	var connected atomic.Bool
	trace := &httptrace.ClientTrace{GotConn: func(httptrace.GotConnInfo) { connected.Store(true) }}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	resp, err := doHTTP(req, proxyURL, nil)
	if err != nil {
		if !connected.Load() {
			return nil, refreshNotConsumed(err)
		}
		return nil, &uncertainRefreshResponseError{detail: "exchange transport failed"}
	}
	return resp, nil
}

func refreshHTTPFailure(ctx context.Context, refresh bool, status int, body []byte, original error) error {
	if !refresh {
		return original
	}
	var payload map[string]json.RawMessage
	parsed := json.Unmarshal(body, &payload) == nil
	if parsed {
		identity, _ := ctx.Value(refreshExchangeIdentityKey{}).(refreshExchangeIdentity)
		for _, field := range []string{"access_token", "refresh_token"} {
			raw := payload[field]
			if len(raw) == 0 || string(raw) == "null" {
				continue
			}
			var token string
			if json.Unmarshal(raw, &token) != nil {
				return &uncertainRefreshResponseError{detail: "ambiguous token fields in error response"}
			}
			if token == "" {
				continue
			}
			knownEcho := field == "access_token" && token == identity.AccessToken || field == "refresh_token" && identity.IssuerScope != "" && store.OAuthRefreshFingerprint(identity.IssuerScope, token) == identity.Fingerprint
			if !knownEcho {
				return &uncertainRefreshResponseError{detail: "ambiguous token fields in error response"}
			}
		}
	}
	var code string
	_ = json.Unmarshal(payload["error"], &code)
	if status >= 400 && status < 500 || parsed && code != "" {
		return refreshNotConsumed(original)
	}
	return &uncertainRefreshResponseError{detail: "unconfirmed exchange rejection"}
}

func refreshPreparationFailure(refresh bool, err error) error {
	if refresh {
		return refreshNotConsumed(err)
	}
	return err
}

type uncertainRefreshResponseError struct{ detail string }

func (e *uncertainRefreshResponseError) Error() string {
	return ErrOAuthRefreshUncertain.Error() + ": " + e.detail
}
func (e *uncertainRefreshResponseError) Unwrap() error      { return ErrOAuthRefreshUncertain }
func (e *uncertainRefreshResponseError) NonRetryable() bool { return true }

// A successful HTTP response can already have rotated the RT. Details come
// only from fixed parser classifications, never provider bodies or I/O errors.
func successfulRefreshResponseError(refresh bool, detail string, original error) error {
	if refresh {
		return &uncertainRefreshResponseError{detail: detail}
	}
	return original
}
