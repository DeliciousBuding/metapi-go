// metapi-go/features/model-tester — template library unit tests.
//
// `loadTesterTemplates` seeds the localStorage-backed preset library on
// first access and keeps returning valid stored templates afterwards;
// corrupt or malformed stores fall back to the presets.

import { describe, expect, it } from 'vitest'

import {
  TESTER_TEMPLATE_PRESETS,
  loadTesterTemplates,
  type TesterTemplate,
} from '../lib/templates'

const STORAGE_KEY = 'model-tester.templates'

type MemoryStorage = {
  getItem: (key: string) => string | null
  setItem: (key: string, value: string) => void
}

function makeStorage(initial?: Record<string, string>): MemoryStorage {
  const data = new Map<string, string>(Object.entries(initial ?? {}))
  return {
    getItem: (key: string) => data.get(key) ?? null,
    setItem: (key: string, value: string) => {
      data.set(key, value)
    },
  }
}

function readStoredTemplates(storage: MemoryStorage): unknown {
  const raw = storage.getItem(STORAGE_KEY)
  return raw ? (JSON.parse(raw) as unknown) : null
}

describe('TESTER_TEMPLATE_PRESETS', () => {
  it('ships five presets with unique ids and i18n keys', () => {
    expect(TESTER_TEMPLATE_PRESETS).toHaveLength(5)
    const ids = TESTER_TEMPLATE_PRESETS.map((template) => template.id)
    expect(new Set(ids).size).toBe(ids.length)
    for (const template of TESTER_TEMPLATE_PRESETS) {
      expect(template.labelKey.startsWith('modelTester.template.')).toBe(true)
      expect(template.promptKey.startsWith('modelTester.template.')).toBe(true)
    }
  })
})

describe('loadTesterTemplates', () => {
  it('seeds the presets into storage on first load', () => {
    const storage = makeStorage()
    const templates = loadTesterTemplates(storage)
    expect(templates).toEqual([...TESTER_TEMPLATE_PRESETS])
    expect(readStoredTemplates(storage)).toEqual([...TESTER_TEMPLATE_PRESETS])
  })

  it('returns stored templates without re-seeding', () => {
    const custom: TesterTemplate[] = [
      {
        id: 'custom',
        labelKey: 'modelTester.template.templates.quickConnectivity.label',
        promptKey: 'modelTester.template.templates.quickConnectivity.prompt',
      },
    ]
    const storage = makeStorage({ [STORAGE_KEY]: JSON.stringify(custom) })
    expect(loadTesterTemplates(storage)).toEqual(custom)
  })

  it('falls back to presets when the stored value is invalid JSON', () => {
    const storage = makeStorage({ [STORAGE_KEY]: '{broken' })
    expect(loadTesterTemplates(storage)).toEqual([...TESTER_TEMPLATE_PRESETS])
  })

  it('falls back to presets when the stored value is not an array', () => {
    const storage = makeStorage({ [STORAGE_KEY]: JSON.stringify({ nope: 1 }) })
    expect(loadTesterTemplates(storage)).toEqual([...TESTER_TEMPLATE_PRESETS])
  })

  it('drops malformed entries when filtering stored templates', () => {
    const storage = makeStorage({
      [STORAGE_KEY]: JSON.stringify([
        { missingKeys: true },
        TESTER_TEMPLATE_PRESETS[0],
      ]),
    })
    expect(loadTesterTemplates(storage)).toEqual([TESTER_TEMPLATE_PRESETS[0]])
  })

  it('returns presets without any storage (no window)', () => {
    expect(loadTesterTemplates()).toEqual([...TESTER_TEMPLATE_PRESETS])
  })
})
