// metapi-go/i18n — language list + detection helpers.
// Skeleton: 2 languages (en + zhCN). Phase 4 will add fr/ru/ja/vi/zhTW.

/**
 * Map a browser-detected locale onto the interface language codes this project
 * uses with i18next (`zhCN`). Browsers report standard BCP-47 tags (`zh-CN`,
 * `zh-Hans`, `zh`, ...) but `supportedLngs`/resources use the camelCase code,
 * so without this mapping a Chinese browser would fall back to English.
 */
export function convertDetectedLanguage(value: string): string {
  const lower = value.trim().replaceAll('_', '-').toLowerCase()
  if (!lower.startsWith('zh')) return value
  return 'zhCN'
}

/**
 * Convert an interface language code into a valid BCP-47 locale tag for the
 * `Intl.*` APIs and the `document.documentElement.lang` attribute.
 * `new Intl.NumberFormat('zhCN')` throws RangeError, so any locale derived
 * from `i18n.language` MUST pass through this first.
 */
export function toBcp47(language: string): string {
  if (language.startsWith('zh') && language.length > 2) {
    return `zh-${language.slice(2)}`
  }
  return language
}

/**
 * Map the active i18next language onto the interface language codes used in
 * `supportedLngs`/resources (`en` / `zhCN`). With `nonExplicitSupportedLngs`
 * enabled, browsers report variants like `en-US` which i18next keeps as-is;
 * this collapses them so UI state (e.g. the switcher check mark) compares
 * against stable codes.
 */
export function normalizeInterfaceLanguage(language: string): string {
  if (language.startsWith('zh')) return 'zhCN'
  const hyphenIndex = language.indexOf('-')
  return hyphenIndex === -1 ? language : language.slice(0, hyphenIndex)
}
