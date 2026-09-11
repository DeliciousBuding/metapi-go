// metapi-go/lib — syncThemeColorMeta: keep <meta name="theme-color"> on the
// resolved --background.
//
// index.html ships a static `#ffffff`, which is wrong the moment the user picks
// dark mode or a colour preset: browsers paint that meta into the address bar,
// the tab strip and the PWA chrome. Both theme providers therefore re-read the
// computed `--background` whenever they change the cascade.
//
// The read happens in an animation frame because a class or data attribute
// written in the same tick has not been styled yet — reading synchronously
// would publish the previous theme's colour.

export function syncThemeColorMeta(): void {
  window.requestAnimationFrame(() => {
    const background = getComputedStyle(document.body)
      .getPropertyValue('--background')
      .trim()
    if (!background) return

    document
      .querySelector('meta[name="theme-color"]')
      ?.setAttribute('content', background)
  })
}
