// Apply the persisted display size before React mounts. Loaded synchronously
// from index.html <body>, so document.body is available without a reflow.
;(function () {
  try {
    function readCookie(name) {
      const match = document.cookie.match(`(?:^|; )${name}=([^;]*)`)
      return match ? decodeURIComponent(match[1]) : ''
    }

    const body = document.body
    const scale = readCookie('theme_scale')
    if (['sm', 'lg', 'xl'].includes(scale)) {
      body.setAttribute('data-theme-scale', scale)
    }
  } catch {
    /* cookie access unavailable — providers apply defaults after boot */
  }
})()
