// Regression test for the zhCN locale crash (#1030): the createdAt column
// must pass the active i18n language through toBcp47() before handing it to
// the Intl-based formatters. `i18n.language` is `zhCN` (not a valid BCP-47
// tag), and `new Intl.DateTimeFormat('zhCN')` throws RangeError, which used
// to crash the whole /sites table for Chinese users.

import '@testing-library/jest-dom/vitest'
import { cleanup, render } from '@testing-library/react'
import type { ReactElement } from 'react'
import {
  afterAll,
  afterEach,
  beforeAll,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import i18n from '@/i18n/config'

import { useSitesColumns } from '../components/sites-columns'
import type { Site } from '../types'

const noopActions = {
  onEdit: () => {},
  onView: () => {},
  onToggleStatus: () => {},
  onTogglePin: () => {},
  onDelete: () => {},
}

function makeSite(overrides: Partial<Site>): Site {
  return {
    id: 1,
    name: 'OpenAI',
    url: 'https://api.openai.com',
    status: 'active',
    isPinned: false,
    ...overrides,
  }
}

function CreatedAtCell({ site }: { site: Site }) {
  const columns = useSitesColumns(noopActions, null)
  const column = columns.find((entry) => entry.id === 'createdAt')
  if (!column?.cell) throw new Error('createdAt column cell missing')
  const cell = column.cell as unknown as (context: {
    row: { original: Site }
  }) => ReactElement
  return cell({ row: { original: site } })
}

beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })

  class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  Object.defineProperty(globalThis, 'ResizeObserver', {
    writable: true,
    value: ResizeObserverStub,
  })
})

afterEach(() => {
  cleanup()
  void i18n.changeLanguage('en')
})

afterAll(() => {
  vi.restoreAllMocks()
})

describe('sites createdAt column locale handling', () => {
  it('renders without RangeError when the language is zhCN', async () => {
    await i18n.changeLanguage('zhCN')
    expect(i18n.language).toBe('zhCN')

    const site = makeSite({
      createdAt: new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString(),
    })

    // Before the fix, `formatRelativeTime(createdAt, 'zhCN')` threw
    // `RangeError: Incorrect locale information provided` here.
    let container: HTMLElement | undefined
    expect(() => {
      container = render(<CreatedAtCell site={site} />).container
    }).not.toThrow()

    const span = container?.querySelector('span[title]')
    // The absolute-date tooltip comes from formatAbsoluteDateTime, which must
    // also survive the zhCN -> zh-CN conversion.
    expect(span?.getAttribute('title')).toBeTruthy()
    expect(span?.textContent).toBeTruthy()
  })

  it('still renders for a plain en locale', () => {
    const site = makeSite({
      createdAt: new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString(),
    })

    expect(() => render(<CreatedAtCell site={site} />)).not.toThrow()
  })
})
