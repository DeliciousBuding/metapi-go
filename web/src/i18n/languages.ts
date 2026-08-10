// metapi-go/i18n — language list + detection helpers.
// Skeleton: 2 languages (en + zhCN). Phase 4 will add fr/ru/ja/vi/zhTW.

export const INTERFACE_LANGUAGE_OPTIONS = [
  { code: 'zhCN', label: '简体中文' },
  { code: 'en', label: 'English' },
] as const

export type InterfaceLanguageCode =
  (typeof INTERFACE_LANGUAGE_OPTIONS)[number]['code']

export function normalizeInterfaceLanguage(value?: string | null): string {
  if (!value) return 'en'

  const normalized = value.trim().replaceAll('_', '-').toLowerCase()
  if (
    value === 'zh-CN' ||
    value === 'zh-Hans' ||
    value === 'zhCN'
  ) {
    return 'zhCN'
  }

  return INTERFACE_LANGUAGE_OPTIONS.some((lang) => lang.code === normalized)
    ? normalized
    : 'en'
}

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
export function toIntlLocale(value?: string | null): string | undefined {
  if (!value) return undefined
  if (value === 'zhCN') return 'zh-CN'
  try {
    return Intl.getCanonicalLocales(value)[0]
  } catch {
    return undefined
  }
}
