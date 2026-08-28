// metapi-go/lib/helpers — outbound proxy URL validation shared by the account,
// site, and OAuth forms.
//
// Mirrors Go `service.IsValidProxyURL` (service/site_endpoint_service.go), the
// validator the create / verify-token handlers enforce: the accepted schemes
// are http, https, socks, socks5 and socks5h. The outbound transport
// (platform/site_proxy.go, `http.ProxyURL`) dials all of these, so the forms
// must not restrict the field to http(s) — doing so wrongly rejected working
// socks5 deployments (issue #1009).
//
// The WHATWG URL parser reads non-special schemes (`socks5:`) with their
// authority intact, so `protocol` / `hostname` behave the same as for http.

/** Proxy schemes the backend accepts (service.IsValidProxyURL). */
const PROXY_SCHEMES = new Set([
  'http:',
  'https:',
  'socks:',
  'socks5:',
  'socks5h:',
])

/**
 * True when `value` is a valid outbound proxy URL. A proxy must parse as a
 * URL with an accepted scheme and a non-empty host — a scheme-only value
 * (`socks5://`) is never dialable, so it is rejected even though the backend
 * `url.Parse` would tolerate it.
 */
export function isProxyUrl(value: string): boolean {
  const trimmed = value.trim()
  try {
    const parsed = new URL(trimmed)
    return PROXY_SCHEMES.has(parsed.protocol) && parsed.hostname !== ''
  } catch {
    return false
  }
}

/**
 * Optional-field variant used by the form schemas: blank means "no proxy" and
 * is always valid; a non-blank value must be a valid proxy URL.
 */
export function isEmptyOrProxyUrl(value: string): boolean {
  return value.trim() === '' || isProxyUrl(value)
}
