

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
 * `Intl.*` APIs. `new Intl.NumberFormat('zhCN')` throws RangeError, so any
 * locale derived from `i18n.language` MUST pass through this first.
 */
