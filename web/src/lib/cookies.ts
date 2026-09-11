// metapi-go/lib — cookie helpers.
//
// UI preferences live in cookies rather than localStorage so the two pre-paint
// bootstraps (`public/bootstrap.js` for the theme class, `public/theme-init.js`
// for preset / font / radius / scale) can read them before the bundle exists.
// That is the only reason this module is cookies and not storage; anything
// without a pre-paint consumer belongs in localStorage.
//
// Hand-written rather than a cookie dependency: the whole surface is three
// one-line `document.cookie` assignments, and the bundle ships inside a single
// Go binary where every kilobyte is paid for on each release.

const SEVEN_DAYS_IN_SECONDS = 60 * 60 * 24 * 7

/**
 * Reads a cookie by exact name, or `undefined` when it is absent.
 *
 * The leading `'; '` normalises the first entry so a single split shape matches
 * every position, and it is also what stops `sidebar_state` from matching a
 * cookie named `mobile_sidebar_state`.
 */
export function getCookie(name: string): string | undefined {
  const [, value] = `; ${document.cookie}`.split(`; ${name}=`)
  return value?.split(';')[0]
}

/** Writes a site-wide cookie expiring after `maxAge` seconds (7 days default). */
export function setCookie(
  name: string,
  value: string,
  maxAge: number = SEVEN_DAYS_IN_SECONDS
): void {
  document.cookie = `${name}=${value}; path=/; max-age=${maxAge}`
}

/**
 * Expires a cookie. `path=/` must match what `setCookie` wrote — a removal
 * against a different path leaves the original cookie in place.
 */
export function removeCookie(name: string): void {
  document.cookie = `${name}=; path=/; max-age=0`
}
