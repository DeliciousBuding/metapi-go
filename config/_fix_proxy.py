src = open('proxy/failure_judge.go', encoding='utf-8').read()
src = src.replace('''func DetectProxyFailure(rawText string, usage *UsageSummary) *FailureResult {
	cfg := config.Get()''',
'''func DetectProxyFailure(rawText string, usage *UsageSummary) *FailureResult {
	rt := config.Runtime()''', 1)
src = src.replace('if len(cfg.ProxyErrorKeywords) > 0 {\n\t\tnormalizedText := strings.ToLower(rawText)\n\t\tfor _, kw := range cfg.ProxyErrorKeywords {',
                  'if len(rt.ProxyErrorKeywords) > 0 {\n\t\tnormalizedText := strings.ToLower(rawText)\n\t\tfor _, kw := range rt.ProxyErrorKeywords {', 1)
src = src.replace('if cfg.ProxyEmptyContentFailEnabled {', 'if rt.ProxyEmptyContentFailEnabled {', 1)
open('proxy/failure_judge.go','w',encoding='utf-8').write(src)

src = open('proxy/session.go', encoding='utf-8').read()
src = src.replace('''type ProxyChannelCoordinator struct {
	cfg *config.Config
}

// NewProxyChannelCoordinator creates a new coordinator.
func NewProxyChannelCoordinator(cfg *config.Config) *ProxyChannelCoordinator {
	return &ProxyChannelCoordinator{
		cfg: cfg,
	}
}''',
'''type ProxyChannelCoordinator struct{}

// NewProxyChannelCoordinator creates a new coordinator. The session-channel
// concurrency limit is read from the runtime-settings snapshot on every
// acquire so a settings change hot-applies without a restart.
func NewProxyChannelCoordinator() *ProxyChannelCoordinator {
	return &ProxyChannelCoordinator{}
}''', 1)
src = src.replace('limit := int(c.cfg.ProxySessionChannelConcurrencyLimit)',
                  'limit := int(config.Runtime().ProxySessionChannelConcurrencyLimit)', 1)
open('proxy/session.go','w',encoding='utf-8').write(src)
print("proxy fixes applied")
