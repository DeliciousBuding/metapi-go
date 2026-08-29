import re

# --- auth/admin.go ---
src = open('auth/admin.go', encoding='utf-8').read()
src = src.replace('''func AdminAuth(cfg *config.Config, sessions *SessionManager) func(http.Handler) http.Handler {
	// Pre-parse the allowlist once at factory creation time so we don't
	// re-parse string entries on every request. Invalid entries are surfaced
	// here (startup time) so operators notice a typo'd allowlist instead of
	// believing the IP restriction is active when it silently dropped.
	parsedAllowlist, invalidEntries := parseAllowlistWithDiagnostics(cfg.AdminIpAllowlist)''',
'''func AdminAuth(sessions *SessionManager) func(http.Handler) http.Handler {
	// Pre-parse the allowlist once at factory creation time so we don't
	// re-parse string entries on every request. Invalid entries are surfaced
	// here (startup time) so operators notice a typo'd allowlist instead of
	// believing the IP restriction is active when it silently dropped.
	// (Allowlist entries are read from the runtime-settings snapshot at
	// wiring time; changing adminIpAllowlist at runtime still requires a
	// restart to take effect, matching pre-C1 behavior.)
	parsedAllowlist, invalidEntries := parseAllowlistWithDiagnostics(config.Runtime().AdminIpAllowlist)''', 1)
src = src.replace('if subtle.ConstantTimeCompare([]byte(token), []byte(cfg.AuthToken)) != 1 {',
                  'if subtle.ConstantTimeCompare([]byte(token), []byte(config.Runtime().AuthToken)) != 1 {', 1)
open('auth/admin.go','w',encoding='utf-8').write(src)

# --- auth/downstream.go ---
src = open('auth/downstream.go', encoding='utf-8').read()
src = src.replace('func AuthorizeDownstreamToken(token string, cfg *config.Config) DownstreamTokenAuthResult {',
                  'func AuthorizeDownstreamToken(token string, rt *config.RuntimeSettings) DownstreamTokenAuthResult {', 1)
src = src.replace('if subtle.ConstantTimeCompare([]byte(normalized), []byte(cfg.ProxyToken)) == 1 {',
                  'if subtle.ConstantTimeCompare([]byte(normalized), []byte(rt.ProxyToken)) == 1 {', 1)
src = src.replace('// 3. Check if token == config.ProxyToken:',
                  '// 3. Check if token == runtime snapshot ProxyToken:', 1)
open('auth/downstream.go','w',encoding='utf-8').write(src)

# --- auth/proxy.go ---
src = open('auth/proxy.go', encoding='utf-8').read()
src = src.replace('func ProxyAuth(cfg *config.Config) func(http.Handler) http.Handler {',
                  'func ProxyAuth() func(http.Handler) http.Handler {', 1)
src = src.replace('result := AuthorizeDownstreamToken(token, cfg)',
                  'result := AuthorizeDownstreamToken(token, config.Runtime())', 1)
src = src.replace('// - Falls back to checking against config.ProxyToken',
                  '// - Falls back to checking against the runtime snapshot ProxyToken', 1)
open('auth/proxy.go','w',encoding='utf-8').write(src)

# --- auth/reauth.go ---
src = open('auth/reauth.go', encoding='utf-8').read()
src = src.replace('''func RequireReauth(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !sensitiveAdminPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			confirm := strings.TrimSpace(r.Header.Get(ReauthConfirmHeader))
			if cfg == nil || confirm == "" || !constantTimeTokenEqual(confirm, cfg.AuthToken) {''',
'''func RequireReauth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !sensitiveAdminPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			confirm := strings.TrimSpace(r.Header.Get(ReauthConfirmHeader))
			rt := config.RuntimeSafe()
			if rt == nil || confirm == "" || !constantTimeTokenEqual(confirm, rt.AuthToken) {''', 1)
open('auth/reauth.go','w',encoding='utf-8').write(src)

# --- routing/status_ranges.go ---
src = open('routing/status_ranges.go', encoding='utf-8').read()
src = src.replace('''	raw := ""
	if cfg := config.GetSafe(); cfg != nil {
		raw = strings.TrimSpace(cfg.ProxyRetryStatusRanges)
	}''',
'''	raw := ""
	if rt := config.RuntimeSafe(); rt != nil {
		raw = strings.TrimSpace(rt.ProxyRetryStatusRanges)
	}''', 1)
src = src.replace('''	raw := ""
	if cfg := config.GetSafe(); cfg != nil {
		raw = strings.TrimSpace(cfg.ProxyDisableStatusRanges)
	}''',
'''	raw := ""
	if rt := config.RuntimeSafe(); rt != nil {
		raw = strings.TrimSpace(rt.ProxyDisableStatusRanges)
	}''', 1)
open('routing/status_ranges.go','w',encoding='utf-8').write(src)
print("auth+routing status_ranges updated")
