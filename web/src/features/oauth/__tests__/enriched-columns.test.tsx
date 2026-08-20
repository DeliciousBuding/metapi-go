// Cell-level regression tests for the OAuth connections enrichment columns
// (quota windows, plan type, route channel count, last model sync error).
// Follows the status-badge-variants harness: the real `useOAuthColumns` hook
// is invoked and each column's `cell` renderer is called with a fixture row,
// so the assertions stay about user-visible output and the em-dash policy.

import '@testing-library/jest-dom/vitest'
import { cleanup, render } from '@testing-library/react'
import type { ReactElement } from 'react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import {
  useOAuthColumns,
  type OAuthColumnActions,
} from '../components/oauth-columns'
import type { OAuthClient } from '../types'

const noopActions: OAuthColumnActions = {
  onRefreshQuota: () => {},
  onRebind: () => {},
  onDelete: () => {},
}

function renderCell(columnId: string, client: Partial<OAuthClient>) {
  function CellHarness() {
    const columns = useOAuthColumns(noopActions)
    const column = columns.find((candidate) => candidate.id === columnId)
    if (!column?.cell) throw new Error(`${columnId} column cell missing`)
    const cell = column.cell as unknown as (context: {
      row: { original: OAuthClient }
    }) => ReactElement
    return cell({ row: { original: client as OAuthClient } })
  }
  return render(<CellHarness />)
}

function buildClient(overrides: Partial<OAuthClient>): Partial<OAuthClient> {
  return {
    accountId: 1,
    siteId: 1,
    provider: 'codex',
    modelCount: 2,
    modelsPreview: [],
    status: 'healthy',
    ...overrides,
  }
}

afterEach(() => cleanup())

describe('oauth quota column', () => {
  it('renders used/limit lines for supported quota windows', () => {
    const { getByText } = renderCell(
      'quota',
      buildClient({
        quota: {
          status: 'supported',
          source: 'official',
          windows: {
            fiveHour: { supported: true, used: 12, limit: 50 },
            sevenDay: { supported: true, used: 30, limit: 350 },
          },
        },
      })
    )

    expect(getByText('12/50')).toBeInTheDocument()
    expect(getByText('30/350')).toBeInTheDocument()
  })

  it('skips unsupported windows and renders used-only windows without a limit', () => {
    const { getByText, queryByText } = renderCell(
      'quota',
      buildClient({
        quota: {
          status: 'supported',
          source: 'reverse_engineered',
          windows: {
            fiveHour: { supported: true, used: 7 },
            sevenDay: { supported: false },
          },
        },
      })
    )

    expect(getByText('7')).toBeInTheDocument()
    expect(queryByText(/7d/)).not.toBeInTheDocument()
  })

  it('renders an em dash when quota is missing, errored, or unsupported', () => {
    for (const client of [
      buildClient({}),
      buildClient({ quota: null }),
      buildClient({
        quota: {
          status: 'unsupported',
          source: 'official',
          windows: {
            fiveHour: { supported: false },
            sevenDay: { supported: false },
          },
        },
      }),
      buildClient({
        quota: {
          status: 'error',
          source: 'official',
          lastError: 'quota sync failed',
          windows: {
            fiveHour: { supported: true, used: 1 },
            sevenDay: { supported: false },
          },
        },
      }),
    ]) {
      const { container, unmount } = renderCell('quota', client)
      expect(container.textContent).toBe('—')
      unmount()
    }
  })
})

describe('oauth planType column', () => {
  it('prefers the connection-level planType', () => {
    const { getByText } = renderCell(
      'planType',
      buildClient({
        planType: 'plus',
        quota: {
          status: 'supported',
          source: 'official',
          subscription: { planType: 'pro' },
          windows: {
            fiveHour: { supported: false },
            sevenDay: { supported: false },
          },
        },
      })
    )

    expect(getByText('plus')).toBeInTheDocument()
  })

  it('falls back to the quota subscription planType', () => {
    const { getByText } = renderCell(
      'planType',
      buildClient({
        quota: {
          status: 'supported',
          source: 'official',
          subscription: { planType: 'pro' },
          windows: {
            fiveHour: { supported: false },
            sevenDay: { supported: false },
          },
        },
      })
    )

    expect(getByText('pro')).toBeInTheDocument()
  })

  it('renders an em dash when no plan is known', () => {
    const { container } = renderCell('planType', buildClient({}))
    expect(container.textContent).toBe('—')
  })
})

describe('oauth routeChannelCount column', () => {
  it('renders the count, including zero', () => {
    const { container, unmount } = renderCell(
      'routeChannelCount',
      buildClient({ routeChannelCount: 4 })
    )
    expect(container.textContent).toBe('4')
    unmount()

    const zeroRender = renderCell(
      'routeChannelCount',
      buildClient({ routeChannelCount: 0 })
    )
    expect(zeroRender.container.textContent).toBe('0')
  })

  it('renders an em dash when the backend omitted the count', () => {
    const { container } = renderCell('routeChannelCount', buildClient({}))
    expect(container.textContent).toBe('—')
  })
})

describe('oauth lastModelSyncError column', () => {
  it('renders a labeled warning icon plus the truncated error with a title', () => {
    const { container, getByTitle } = renderCell(
      'lastModelSyncError',
      buildClient({ lastModelSyncError: 'model list fetch failed: 503' })
    )

    const icon = container.querySelector('svg')
    expect(icon).not.toBeNull()
    expect(icon?.getAttribute('aria-label')).toBe('Sync error')
    expect(getByTitle('model list fetch failed: 503')).toBeInTheDocument()
  })

  it('renders an em dash when the last sync had no error', () => {
    const { container } = renderCell(
      'lastModelSyncError',
      buildClient({ lastModelSyncError: null })
    )
    expect(container.textContent).toBe('—')
  })
})
