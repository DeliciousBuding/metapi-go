// metapi-go/config — font registry for the font-provider.
// metapi uses Public Sans (humanist sans) + Lora (editorial serif),
// both bundled via @fontsource-variable. The font-provider toggles the
// `font-<name>` class on <html>; Tailwind maps these to font families
// declared in theme.css.

export const fonts = ['sans', 'serif'] as const
