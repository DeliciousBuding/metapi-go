// FOUC bootstrap: resolve theme BEFORE first paint to avoid flash.
// Loaded synchronously from index.html <head> (no defer/async) so it runs
// before the first frame, exactly like the former inline script. Must stay
// dependency-free: it executes before any bundled module loads.
//
// Cookie-based (vite-ui-theme, 1y) to align with newapi next-themes cookie
// storage; falls back to system preference. Legacy themeBootstrap.ts removed.
// Tailwind 4 dark mode is class-based: <html class="dark"> / class="light".
;(function () {
  try {
    const COOKIE_NAME = 'vite-ui-theme'
    let theme = null
    // Read theme from cookie (matches next-themes cookieStorage convention)
    const match = document.cookie.match('(?:^|; )' + COOKIE_NAME + '=([^;]*)')
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
