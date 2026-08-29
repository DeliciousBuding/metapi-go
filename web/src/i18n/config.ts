// metapi-go/i18n — i18next configuration.
// Skeleton: 2 languages (en + zhCN), key-based translation, localStorage
// detection. nsSeparator disabled so literal colons in keys are preserved;
// nested objects still traverse via keySeparator ('.').
//
// Locale bundles are lazy-loaded through a tiny i18next backend so the entry
// chunk no longer statically carries both en + zh-CN JSON (~120KB). The active
// language loads on init (main.tsx awaits initI18n before first paint); the
// sibling loads on first switch.

import i18n, { type BackendModule, type ResourceLanguage } from 'i18next'
import LanguageDetector from 'i18next-browser-languagedetector'
import { initReactI18next } from 'react-i18next'

import { convertDetectedLanguage, toBcp47 } from './languages'

// webpackChunkName pins stable semantic chunk names (locale-en / locale-zh-CN)
// so the two locale bundles are individually visible in size reports and their
// content-hashed URLs never depend on anonymous chunk ids.
const localeLoaders = {
  en: () => import(/* webpackChunkName: 'locale-en' */ './locales/en.json'),
  zhCN: () =>
    import(/* webpackChunkName: 'locale-zh-CN' */ './locales/zh-CN.json'),
} as const

function normalizeLanguage(language: string): keyof typeof localeLoaders {
  return language.startsWith('zh') ? 'zhCN' : 'en'
}

const localeBackend: BackendModule = {
  type: 'backend',
  init() {},
  async read(language, namespace, callback) {
    if (namespace !== 'translation') {
      callback(null, {})
      return
    }
    try {
      const module = await localeLoaders[normalizeLanguage(language)]()
      callback(null, module.default.translation as ResourceLanguage)
    } catch (error) {
      callback(error as Error, null)
    }
  },
}

i18n.use(localeBackend).use(LanguageDetector).use(initReactI18next)

/**
 * Initialize i18next. The returned promise resolves once the active language
 * bundle is loaded, so callers that gate rendering on it never paint
 * untranslated keys.
 */
export function initI18n(): Promise<unknown> {
  return i18n.init({
    fallbackLng: 'en',
    supportedLngs: ['en', 'zhCN'],
    // Treat browser variants like `en-US` as supported (they resolve to the
    // base `en` resources via the fallback chain).
    nonExplicitSupportedLngs: true,
    load: 'currentOnly',
    nsSeparator: false,
    // i18next's debug logging is noisy under vitest; keep it for dev only.
    debug: import.meta.env.DEV && !import.meta.env.TEST,
    interpolation: {
      escapeValue: false,
    },
    detection: {
      order: ['localStorage', 'navigator'],
      caches: ['localStorage'],
      convertDetectedLanguage,
    },
  })
}

/**
 * Keep `<html lang>` in sync with the active language so assistive tech,
 * spell checkers and locale-aware tooling see the current BCP-47 tag.
 * `dir` is intentionally NOT touched here — `DirectionProvider` is the single
 * owner of the direction attribute (a language change must not clobber an
 * RTL user's direction cookie).
 */
function syncDocumentLanguage(language: string): void {
  document.documentElement.lang = toBcp47(language)
}

i18n.on('languageChanged', syncDocumentLanguage)
// The language may not be resolved yet at module load (async init); the
// `languageChanged` event re-syncs once init settles.
syncDocumentLanguage(i18n.language || i18n.resolvedLanguage || 'en')

export default i18n
