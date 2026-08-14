package platform

// DefaultBrowserUserAgent is the single fixed browser identity used for
// outbound requests to upstream sites. Adapter API calls (login/checkin/
// balance/model pull) previously went out as Go's default "Go-http-client/1.1",
// which is a bright bot beacon for UA-denial WAF rules on NewAPI/OneAPI forks.
//
// Keep the Chrome version bumped quarterly so the declared identity stays
// current with the real Chrome release cadence.
const DefaultBrowserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
