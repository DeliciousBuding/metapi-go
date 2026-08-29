// FOUC bootstrap: resolve theme BEFORE first paint to avoid flash.
// Loaded synchronously from index.html <head> (no defer/async) so it runs
// before the first frame, exactly like the former inline script. Must stay
// dependency-free: it executes before any bundled module loads.
//
// Cookie-based (vite-ui-theme, 1y) to align with newapi next-themes cookie
// storage; falls back to system preference. Legacy themeBootstrap.ts removed.
// Tailwind 4 dark mode is class-based: <html class="dark"> / class="light".
;(function () {
  // CSP runtime-style handshake (#1035 S2): the Go SPA fallback injects a
  // per-request nonce into <meta name="csp-nonce"> in <head> before this
  // script runs. Hand it to every subsequently-created <style> element so
  // runtime style injectors (sonner's stylesheet, chart color variables,
  // dialog/command-palette scroll lock) carry the required nonce attribute
  // from the moment they are inserted — including libraries that append an
  // empty <style> before filling it. Also publish __webpack_nonce__ for
  // get-nonce/style-singleton consumers. Does nothing when served without a
  // CSP (dev server), preserving the no-nonce behavior there.
  try {
    const nonceMeta = document.querySelector('meta[name="csp-nonce"]')
    const cspNonce = nonceMeta ? nonceMeta.content : ''
    if (cspNonce) {
      try {
        window.__webpack_nonce__ = cspNonce
      } catch {
        /* frozen global in exotic embeds — style tag patch below still works */
      }
      const originalCreateElement = document.createElement.bind(document)
      document.createElement = function (tagName, options) {
        const el = originalCreateElement(tagName, options)
        if (
          String(tagName).toLowerCase() === 'style' &&
          !el.hasAttribute('nonce')
        ) {
          el.setAttribute('nonce', cspNonce)
        }
        return el
      }
    }
  } catch {
    /* nonce unavailable — leave runtime style behavior unchanged */
  }
  try {
    const COOKIE_NAME = 'vite-ui-theme'
    let theme = null
    // Read theme from cookie (matches next-themes cookieStorage convention)
    const match = document.cookie.match(`(?:^|; )${COOKIE_NAME}=([^;]*)`)
    const stored = match ? decodeURIComponent(match[1]) : ''
    if (stored === 'light' || stored === 'dark') {
      theme = stored
    } else if (stored === 'system' || !stored) {
      theme = window.matchMedia('(prefers-color-scheme: dark)').matches
        ? 'dark'
        : 'light'
    }
    const root = document.documentElement
    root.classList.remove('light', 'dark')
    root.classList.add(theme)
    root.style.colorScheme = theme
    // Use the design token as soon as CSS is available, with a matching
    // light/dark fallback before the stylesheet finishes loading.
    const bg = theme === 'dark' ? '#1d1d1d' : '#ffffff'
    root.style.setProperty('--bootstrap-background', bg)
    root.style.backgroundColor =
      'var(--background, var(--bootstrap-background))'
    const themeColor = document.querySelector('meta[name="theme-color"]')
    if (themeColor) themeColor.setAttribute('content', bg)
  } catch {
    /* cookie / matchMedia unavailable — leave browser defaults */
  }
})()
