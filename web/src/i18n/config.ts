// metapi-go/i18n — i18next configuration.
// Skeleton: 2 languages (en + zhCN), key-based translation, localStorage
// detection. nsSeparator disabled so literal colons in keys are preserved;
// nested objects still traverse via keySeparator ('.').

import i18n from 'i18next'
import LanguageDetector from 'i18next-browser-languagedetector'
import { initReactI18next } from 'react-i18next'

import { convertDetectedLanguage } from './languages'
import en from './locales/en.json'
import zhCN from './locales/zh-CN.json'

export const resources = {
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
    load: 'currentOnly',
    nsSeparator: false,
    debug: import.meta.env.DEV,
    interpolation: {
      escapeValue: false,
    },
    detection: {
      order: ['localStorage', 'navigator'],
      caches: ['localStorage'],
      convertDetectedLanguage,
    },
  })

export default i18n
