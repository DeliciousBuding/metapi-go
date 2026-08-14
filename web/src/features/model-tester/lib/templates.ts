// metapi-go/features/model-tester — local template library (localStorage).
//
// The tester ships with a small preset library (quick connectivity, format
// conversion, long context, tool calling, chain-of-thought reasoning).
// Presets are seeded into localStorage on first load so the library is
// purely local (no backend dependency) and can later grow user-authored
// templates without a wire change. Template prompts live in the i18n
// dictionaries so the injected prompt matches the active UI language; the
// stored records only carry i18n keys, never resolved text.

export type TesterTemplate = {
  id: string
  labelKey: string
  promptKey: string
  temperature?: number
  topP?: number
  maxTokens?: number
}

const TEMPLATES_STORAGE_KEY = 'model-tester.templates'

export const TESTER_TEMPLATE_PRESETS: readonly TesterTemplate[] = [
  {
    id: 'quick-connectivity',
    labelKey: 'modelTester.template.templates.quickConnectivity.label',
    promptKey: 'modelTester.template.templates.quickConnectivity.prompt',
    temperature: 0,
    maxTokens: 64,
  },
  {
    id: 'format-conversion',
    labelKey: 'modelTester.template.templates.formatConversion.label',
    promptKey: 'modelTester.template.templates.formatConversion.prompt',
    temperature: 0.2,
    maxTokens: 1024,
  },
  {
    id: 'long-context',
    labelKey: 'modelTester.template.templates.longContext.label',
    promptKey: 'modelTester.template.templates.longContext.prompt',
    temperature: 0.3,
    maxTokens: 2048,
  },
  {
    id: 'tool-calling',
    labelKey: 'modelTester.template.templates.toolCalling.label',
    promptKey: 'modelTester.template.templates.toolCalling.prompt',
    temperature: 0,
    maxTokens: 512,
  },
  {
    id: 'chain-of-thought',
    labelKey: 'modelTester.template.templates.chainOfThought.label',
    promptKey: 'modelTester.template.templates.chainOfThought.prompt',
    temperature: 0.6,
    topP: 0.9,
    maxTokens: 2048,
  },
]

type TemplateStorage = Pick<Storage, 'getItem' | 'setItem'>

function isTesterTemplate(value: unknown): value is TesterTemplate {
  if (!value || typeof value !== 'object') return false
  const record = value as Record<string, unknown>
  return (
    typeof record.id === 'string' &&
    typeof record.labelKey === 'string' &&
    typeof record.promptKey === 'string'
  )
}

function readStoredTemplates(storage?: TemplateStorage): TesterTemplate[] {
  if (!storage) return []
  try {
    const raw = storage.getItem(TEMPLATES_STORAGE_KEY)
    if (!raw) return []
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.filter(isTesterTemplate)
  } catch {
    return []
  }
}

/**
 * Load the template library: stored templates win; on first visit (or a
 * corrupt/empty store) the presets are seeded into localStorage. Callers
 * may inject storage (tests); `defaultTemplateStorage` resolves the
 * browser's localStorage.
 */
export function loadTesterTemplates(
  storage?: TemplateStorage
): TesterTemplate[] {
  const stored = readStoredTemplates(storage)
  if (stored.length > 0) return stored
  if (storage) {
    try {
      storage.setItem(
        TEMPLATES_STORAGE_KEY,
        JSON.stringify(TESTER_TEMPLATE_PRESETS)
      )
    } catch {
      // Storage unavailable (quota/denied) — fall back to the in-memory presets.
    }
  }
  return [...TESTER_TEMPLATE_PRESETS]
}

/** Browser localStorage when available (jsdom/tests may omit it). */
export function defaultTemplateStorage(): TemplateStorage | undefined {
  if (typeof window === 'undefined') return undefined
  return window.localStorage
}
