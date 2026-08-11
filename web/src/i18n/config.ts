// metapi-go/i18n — i18next configuration.
// Skeleton: 2 languages (en + zhCN), key-based translation, localStorage
// detection. nsSeparator disabled so literal colons in keys are preserved;
// nested objects still traverse via keySeparator ('.').

import i18n from 'i18next'
import LanguageDetector from 'i18next-browser-languagedetector'
import { initReactI18next } from 'react-i18next'

import { convertDetectedLanguage, toBcp47 } from './languages'
import en from './locales/en.json'
import zhCN from './locales/zh-CN.json'

const resources = {
  en,
  zhCN,
} as const

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources,
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

/**
 * Keep `<html lang>` in sync with the active language so assistive tech,
 * spell checkers and locale-aware tooling see the current BCP-47 tag. Both
 * supported languages are LTR, so dir stays pinned to ltr.
 */
function syncDocumentLanguage(language: string): void {
  document.documentElement.lang = toBcp47(language)
  document.documentElement.dir = 'ltr'
}

i18n.on('languageChanged', syncDocumentLanguage)
// The language may not be resolved yet at module load (async init); the
// `languageChanged` event re-syncs once init settles.
syncDocumentLanguage(i18n.language || i18n.resolvedLanguage || 'en')

export default i18n
