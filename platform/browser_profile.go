package platform

import "net/http"

// DefaultBrowserUserAgent is the single fixed browser identity used for
// outbound requests to upstream sites. Adapter API calls (login/checkin/
// balance/model pull) previously went out as Go's default "Go-http-client/1.1",
// which is a bright bot beacon for UA-denial WAF rules on NewAPI/OneAPI forks.
//
// Keep the Chrome version bumped quarterly so the declared identity stays
// current with the real Chrome release cadence.
const DefaultBrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// ApplySiteIdentity injects a per-site Cloudflare clearance cookie and browser
// UA override into the request. The Cookie header is deny-listed in
// custom_headers for security, so cf_clearance flows through this dedicated
// typed field instead. Existing cookies are preserved via AddCookie.
func ApplySiteIdentity(req *http.Request, proxyConfig *ProxyConfig) {
	if proxyConfig == nil {
		return
	}
	if proxyConfig.ClearanceCookie != "" {
		req.AddCookie(&http.Cookie{Name: "cf_clearance", Value: proxyConfig.ClearanceCookie})
	}
	if proxyConfig.BrowserUA != "" {
		req.Header.Set("User-Agent", proxyConfig.BrowserUA)
	}
}
