package auth

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/trustedproxy"
	"github.com/deliciousbuding/metapi-go/store"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/time/rate"
)

const UserSessionCookieName = "metapi_user_session"
const UserSessionTTL = 24 * time.Hour
const UserSessionIdleTTL = 2 * time.Hour
const UserActivationTTL = 30 * time.Minute

var ErrUserCredentials = errors.New("invalid user credentials")
var ErrUserPassword = errors.New("password must contain 12 to 72 bytes")

// UserSessionManager has no administrator capabilities. Random credentials
// carry 256 bits; only their SHA-256 digests persist. Passwords use bcrypt12.
type UserSessionManager struct {
	store    *store.UserAuthStore
	accounts *ipRateLimiter
}

func NewUserSessionManager(db *store.DB) *UserSessionManager {
	if db == nil {
		return nil
	}
	accounts := newIPRateLimiter(1, 5)
	accounts.rps = rate.Every(10 * time.Second)
	return &UserSessionManager{store: store.NewUserAuthStore(db), accounts: accounts}
}

func (m *UserSessionManager) IssueActivation(ctx context.Context, userID int64) (string, time.Time, error) {
	if m == nil {
		return "", time.Time{}, store.ErrGatewayUserUnavailable
	}
	code, err := randomHex(32)
	if err != nil {
		return "", time.Time{}, err
	}
	version, err := randomHex(32)
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().UTC().Add(UserActivationTTL)
	err = m.store.IssueActivation(ctx, userID, hashSessionToken(code), version, time.Now(), expires)
	if err != nil {
		return "", time.Time{}, err
	}
	return code, expires, nil
}

func userPasswordHash(password string) (string, error) {
	if len(password) < 12 || len(password) > 72 {
		return "", ErrUserPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(hash), err
}

// AllowAccount bounds slow-hash work per canonical username as well as the
// router's per-IP bucket. Unknown names follow the same bucket and response.
func (m *UserSessionManager) AllowAccount(username string) bool {
	return m != nil && m.accounts.allow(strings.ToLower(strings.TrimSpace(username)))
}

func (m *UserSessionManager) Activate(ctx context.Context, username, code, password string) error {
	if m == nil {
		return store.ErrGatewayUserUnavailable
	}
	if len(code) != 64 {
		return ErrUserCredentials
	}
	hash, err := userPasswordHash(password)
	if err != nil {
		return err
	}
	version, err := randomHex(32)
	if err != nil {
		return err
	}
	err = m.store.Activate(ctx, strings.ToLower(strings.TrimSpace(username)), hashSessionToken(code), hash, version, time.Now())
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUserCredentials
	}
	return err
}

// A fixed valid cost12 hash ensures unknown/disabled/unactivated accounts do
// comparable password work instead of disclosing their existence by timing.
const dummyUserHash = "$2a$12$R9h/cIPz0gi.URNNX3kh2OPST9/PgBkqquzi.Ss7KIUgO2t0jWMUW"

func (m *UserSessionManager) Login(ctx context.Context, username, password string) (string, *store.UserSession, error) {
	if m == nil {
		return "", nil, store.ErrGatewayUserUnavailable
	}
	credential, err := m.store.Credential(ctx, strings.ToLower(strings.TrimSpace(username)))
	hash := credential.PasswordHash
	if err != nil || hash == "" {
		hash = dummyUserHash
	}
	passwordErr := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", nil, err
	}
	if err != nil || credential.PasswordHash == "" || passwordErr != nil {
		return "", nil, ErrUserCredentials
	}
	raw, err := randomHex(32)
	if err != nil {
		return "", nil, err
	}
	now := time.Now().UTC()
	expires := now.Add(UserSessionTTL)
	err = m.store.CreateSession(ctx, credential, hashSessionToken(raw), now, expires, now.Add(UserSessionIdleTTL))
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, ErrUserCredentials
	}
	if err != nil {
		return "", nil, err
	}
	return raw, &store.UserSession{UserID: credential.UserID, ExpiresAt: expires.Format(time.RFC3339Nano)}, nil
}

func (m *UserSessionManager) ChangePassword(ctx context.Context, userID int64, username, currentPassword, newPassword string) error {
	if m == nil {
		return store.ErrGatewayUserUnavailable
	}
	credential, err := m.store.Credential(ctx, username)
	if err != nil {
		return err
	}
	if credential.UserID != userID || bcrypt.CompareHashAndPassword([]byte(credential.PasswordHash), []byte(currentPassword)) != nil {
		return ErrUserCredentials
	}
	hash, err := userPasswordHash(newPassword)
	if err != nil {
		return err
	}
	version, err := randomHex(32)
	if err != nil {
		return err
	}
	return m.store.ChangePassword(ctx, userID, credential.Version, hash, version, time.Now())
}

func (m *UserSessionManager) Revoke(ctx context.Context, raw string) error {
	if m == nil {
		return store.ErrGatewayUserUnavailable
	}
	return m.store.RevokeSession(ctx, hashSessionToken(raw))
}

type userSessionKey struct{}

func GetUserSession(ctx context.Context) *store.UserSession {
	v, _ := ctx.Value(userSessionKey{}).(*store.UserSession)
	return v
}

func UserSessionAuth(m *UserSessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			noStore(w)
			if m == nil {
				writeJSON(w, 503, jsonError("User sessions unavailable"))
				return
			}
			cookie, err := r.Cookie(UserSessionCookieName)
			if err != nil || len(cookie.Value) != 64 {
				writeJSON(w, 401, jsonError("User login required"))
				return
			}
			session, err := m.store.ValidateSession(r.Context(), hashSessionToken(cookie.Value), time.Now(), UserSessionIdleTTL)
			if err != nil {
				status := 503
				if errors.Is(err, sql.ErrNoRows) {
					status = 401
				}
				writeJSON(w, status, jsonError("User session unavailable"))
				return
			}
			ctx := WithGatewayUserPrincipal(r.Context(), GatewayUserPrincipal{UserID: session.UserID})
			ctx = context.WithValue(ctx, userSessionKey{}, &session)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func SetUserSessionCookie(w http.ResponseWriter, r *http.Request, raw, secureMode string) {
	http.SetCookie(w, &http.Cookie{Name: UserSessionCookieName, Value: raw, Path: "/api/user", MaxAge: int(UserSessionTTL.Seconds()), HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: cookieSecure(r, secureMode)})
}
func ClearUserSessionCookie(w http.ResponseWriter, r *http.Request, secureMode string) {
	http.SetCookie(w, &http.Cookie{Name: UserSessionCookieName, Value: "", Path: "/api/user", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: cookieSecure(r, secureMode)})
}

// Exact scheme+host+port comparison: an HTTP sibling origin is not HTTPS,
// even when its hostname matches. Forwarded protocol is trusted only through
// the existing trusted-proxy boundary. Headerless non-browser clients work.
func UserSameOriginGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
			scheme := "http"
			if trustedproxy.IsHTTPS(r) {
				scheme = "https"
			}
			if !strings.EqualFold(origin, scheme+"://"+r.Host) {
				writeCrossSiteRejected(w)
				return
			}
		} else {
			switch strings.ToLower(r.Header.Get("Sec-Fetch-Site")) {
			case "cross-site", "same-site":
				writeCrossSiteRejected(w)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
