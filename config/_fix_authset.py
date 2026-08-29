src = open('handler/admin/auth_settings.go', encoding='utf-8').read()
src = src.replace('''func RegisterAuthSettingsRoutes(r chi.Router, db *sqlx.DB, cfg *config.Config, sessions *auth.SessionManager) {
	handler := &authSettingsHandler{db: db, cfg: cfg, sessions: sessions}''',
'''func RegisterAuthSettingsRoutes(r chi.Router, db *sqlx.DB, sessions *auth.SessionManager) {
	handler := &authSettingsHandler{db: db, sessions: sessions}''', 1)
src = src.replace('''type authSettingsHandler struct {
	db       *sqlx.DB
	cfg      *config.Config
	sessions *auth.SessionManager
}''',
'''type authSettingsHandler struct {
	db       *sqlx.DB
	sessions *auth.SessionManager
}''', 1)
src = src.replace('	token := h.cfg.AuthToken', '	token := config.Runtime().AuthToken', 1)
src = src.replace('if !constantTimeTokenEqual(body.OldToken, h.cfg.AuthToken) {',
                  'if !constantTimeTokenEqual(body.OldToken, config.Runtime().AuthToken) {', 1)
src = src.replace('''	// Update runtime config
	h.cfg.AuthToken = body.NewToken''',
'''	// Publish the rotated token atomically; every subsequent admin/proxy
	// auth check sees the new value without a restart.
	config.UpdateRuntime(func(r *config.RuntimeSettings) { r.AuthToken = body.NewToken })''', 1)
open('handler/admin/auth_settings.go','w',encoding='utf-8').write(src)
print("auth_settings updated")
