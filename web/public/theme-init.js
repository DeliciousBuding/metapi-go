// Theme customization bootstrap: apply cookie-backed preset/font/radius/scale
// as body data attributes before React mounts, so the first render already
// carries the user's customization (no post-mount reflow). Loaded
// synchronously from index.html <body> (no defer/async), exactly like the
// former inline script, so document.body is available when it runs.
;(function () {
  try {
    function readCookie(name) {
      const match = document.cookie.match(`(?:^|; )${name}=([^;]*)`)
      return match ? decodeURIComponent(match[1]) : ''
    }

    const body = document.body
    const preset = readCookie('theme_preset')
    const presets = [
      'anthropic',
      'simple-large',
      'underground',
      'rose-garden',
      'lake-view',
      'sunset-glow',
      'forest-whisper',
      'ocean-breeze',
      'lavender-dream',
    ]
    if (presets.includes(preset)) {
      body.setAttribute('data-theme-preset', preset)
    }

    const font = readCookie('theme_font')
    body.setAttribute('data-theme-font', font === 'serif' ? 'serif' : 'sans')

    const radius = readCookie('theme_radius')
    if (['none', 'sm', 'md', 'lg', 'xl'].includes(radius)) {
      body.setAttribute('data-theme-radius', radius)
    }

    const scale = readCookie('theme_scale')
    if (['sm', 'lg', 'xl'].includes(scale)) {
      body.setAttribute('data-theme-scale', scale)
    }

    const contentLayout = readCookie('theme_content_layout')
    body.setAttribute(
      'data-theme-content-layout',
      contentLayout === 'centered' ? 'centered' : 'full'
    )

    window.requestAnimationFrame(function () {
      const background = getComputedStyle(body)
        .getPropertyValue('--background')
        .trim()
      const themeColor = document.querySelector('meta[name="theme-color"]')
      if (background && themeColor) {
        themeColor.setAttribute('content', background)
      }
    })
  } catch {
    /* cookie access unavailable — providers apply defaults after boot */
  }
})()
