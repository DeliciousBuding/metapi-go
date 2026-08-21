// Behavior tests for the OAuth connection detail sheet (issue #887 S4:
// OAuth was the only list page in the console without a detail sheet).
//
// The assertions are deliberately about what an operator SEES, with the
// honesty policy as the centrepiece: quota numbers the provider never sent
// must render an explicit sentence, never a `0` (a fabricated "0 / 0" reads
// as "nothing used" or "quota exhausted" depending on the column, and both
// are lies). The footer actions are asserted to call the page-owned
// mutations rather than instantiating their own.

import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import type { OAuthClient } from '../../types'
import { OAuthDetailSheet } from '../oauth-detail-sheet'

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
})

afterEach(() => cleanup())

function buildConnection(overrides: Partial<OAuthClient>): OAuthClient {
  return {
    accountId: 42,
    siteId: 7,
    provider: 'codex',
    username: 'ops@example.com',
    modelCount: 3,
    modelsPreview: ['gpt-5', 'gpt-5-mini'],
    status: 'healthy',
    ...overrides,
  } as OAuthClient
}

function renderSheet(
  connection: OAuthClient | null,
  overrides?: {
    onRefreshQuota?: (connection: OAuthClient) => void
    onRebind?: (connection: OAuthClient) => void
    isRefreshingQuota?: boolean
    isRebinding?: boolean
  }
) {
  return render(
    <OAuthDetailSheet
      connection={connection}
      open
      onOpenChange={() => {}}
      onRefreshQuota={overrides?.onRefreshQuota ?? (() => {})}
      onRebind={overrides?.onRebind ?? (() => {})}
      isRefreshingQuota={overrides?.isRefreshingQuota ?? false}
      isRebinding={overrides?.isRebinding ?? false}
    />
  )
}

describe('OAuthDetailSheet overview section', () => {
  it('renders the contract fields the list row cannot show', () => {
    renderSheet(
      buildConnection({
        projectId: 'proj-9001',
        planType: 'pro',
        site: {
          id: 7,
          name: 'Codex upstream',
          url: 'https://codex.example.com',
          platform: 'codex',
        },
      })
    )

    // The display label doubles as the sheet title and the username field.
    expect(
      screen.getByRole('heading', { name: 'ops@example.com' })
    ).toBeInTheDocument()
    expect(screen.getAllByText('ops@example.com')).toHaveLength(2)
    expect(screen.getByText('codex')).toBeInTheDocument()
    expect(screen.getByText('proj-9001')).toBeInTheDocument()
    expect(screen.getByText('pro')).toBeInTheDocument()
    expect(screen.getByText('Codex upstream')).toBeInTheDocument()
    expect(screen.getByText('#42')).toBeInTheDocument()
    expect(screen.getByText('Healthy')).toBeInTheDocument()
  })

  it('falls back to the site id when the projection carries no site object', () => {
    renderSheet(buildConnection({ site: null }))

    expect(screen.getByText('#7')).toBeInTheDocument()
  })

  it('renders an em dash for an absent projectId instead of an empty cell', () => {
    // The sheet renders through a portal, so assertions read `baseElement`
    // (document.body), not the render container.
    const { baseElement } = renderSheet(
      buildConnection({ projectId: null, planType: null })
    )

    const emDashes = [...baseElement.querySelectorAll('dd')].filter(
      (definition) => definition.textContent === '—'
    )
    expect(emDashes.length).toBeGreaterThanOrEqual(2)
  })

  it('renders the abnormal status through the destructive badge variant', () => {
    const { baseElement } = renderSheet(buildConnection({ status: 'abnormal' }))

    const badge = baseElement.querySelector('[data-slot="badge"]')
    expect(badge?.getAttribute('data-variant')).toBe('destructive')
  })
})

describe('OAuthDetailSheet quota section', () => {
  it('renders used / limit / remaining / resetAt for both supported windows', () => {
    renderSheet(
      buildConnection({
        quota: {
          status: 'supported',
          source: 'official',
          windows: {
            fiveHour: {
              supported: true,
              used: 12,
              limit: 50,
              remaining: 38,
              resetAt: '2026-08-21T18:00:00Z',
            },
            sevenDay: {
              supported: true,
              used: 300,
              limit: 900,
              remaining: 600,
              resetAt: '2026-08-27T00:00:00Z',
            },
          },
        },
      })
    )

    expect(screen.getByText('5-hour window')).toBeInTheDocument()
    expect(screen.getByText('7-day window')).toBeInTheDocument()
    // `remaining` and `resetAt` exist in the contract but never fit the list
    // column — the sheet is the only place they surface.
    expect(screen.getAllByText('Remaining')).toHaveLength(2)
    expect(screen.getAllByText('Resets at')).toHaveLength(2)
    expect(screen.getByText('38')).toBeInTheDocument()
    expect(screen.getByText('600')).toBeInTheDocument()
    expect(screen.getByText('12')).toBeInTheDocument()
    expect(screen.getByText('900')).toBeInTheDocument()
  })

  it('states that an unsupported provider reports no quota instead of showing zeros', () => {
    const { baseElement } = renderSheet(
      buildConnection({
        quota: {
          status: 'unsupported',
          source: 'official',
          windows: {
            fiveHour: { supported: false },
            sevenDay: { supported: false },
          },
        },
      })
    )

    expect(
      screen.getByText('This provider does not expose quota data upstream.')
    ).toBeInTheDocument()
    expect(screen.getAllByText('Not reported by upstream.')).toHaveLength(2)
    // No fabricated numbers anywhere in the quota section.
    expect(baseElement.textContent).not.toContain('Remaining')
    expect(screen.queryByText('0')).not.toBeInTheDocument()
  })

  it('states that a supported-but-empty window has no numbers yet', () => {
    renderSheet(
      buildConnection({
        quota: {
          status: 'supported',
          source: 'reverse_engineered',
          windows: {
            fiveHour: { supported: true },
            sevenDay: { supported: true, used: 5, limit: 10 },
          },
        },
      })
    )

    expect(
      screen.getByText(
        'Supported, but the provider has not reported any numbers yet.'
      )
    ).toBeInTheDocument()
    expect(screen.queryByText('0')).not.toBeInTheDocument()
  })

  it('explains a missing quota payload instead of rendering an empty section', () => {
    renderSheet(buildConnection({ quota: null }))

    expect(
      screen.getByText(
        'The provider has not reported any quota data for this connection yet.'
      )
    ).toBeInTheDocument()
    expect(screen.queryByText('5-hour window')).not.toBeInTheDocument()
  })

  it('surfaces the upstream error text when the last quota sync failed', () => {
    renderSheet(
      buildConnection({
        quota: {
          status: 'error',
          source: 'official',
          lastError: 'quota endpoint returned 503',
          windows: {
            fiveHour: { supported: true, used: 4, limit: 50 },
            sevenDay: { supported: false },
          },
        },
      })
    )

    expect(
      screen.getByText(
        'The last quota sync failed, so the values below may be missing or stale.'
      )
    ).toBeInTheDocument()
    expect(screen.getByText('quota endpoint returned 503')).toBeInTheDocument()
  })

  it('renders the subscription window and hides it when every field is empty', () => {
    const withSubscription = renderSheet(
      buildConnection({
        quota: {
          status: 'supported',
          source: 'official',
          subscription: {
            planType: 'pro',
            activeStart: '2026-08-01T00:00:00Z',
            activeUntil: '2026-09-01T00:00:00Z',
          },
          lastLimitResetAt: '2026-08-20T00:00:00Z',
          windows: {
            fiveHour: { supported: true, used: 1, limit: 50 },
            sevenDay: { supported: false },
          },
        },
      })
    )
    expect(screen.getByText('Subscription plan')).toBeInTheDocument()
    expect(screen.getByText('Active until')).toBeInTheDocument()
    expect(screen.getByText('Last limit reset')).toBeInTheDocument()
    withSubscription.unmount()

    renderSheet(
      buildConnection({
        quota: {
          status: 'supported',
          source: 'official',
          subscription: {
            planType: null,
            activeStart: null,
            activeUntil: null,
          },
          windows: {
            fiveHour: { supported: true, used: 1, limit: 50 },
            sevenDay: { supported: false },
          },
        },
      })
    )
    expect(screen.queryByText('Subscription plan')).not.toBeInTheDocument()
  })
})

describe('OAuthDetailSheet models and sync sections', () => {
  it('labels the model list as a preview with the shown/total counts', () => {
    renderSheet(
      buildConnection({ modelCount: 3, modelsPreview: ['gpt-5', 'gpt-5-mini'] })
    )

    expect(
      screen.getByText('Preview only — 2 of 3 discovered models.')
    ).toBeInTheDocument()
    expect(screen.getByText('gpt-5')).toBeInTheDocument()
    expect(screen.getByText('gpt-5-mini')).toBeInTheDocument()
  })

  it('states that no models were discovered instead of rendering an empty list', () => {
    renderSheet(buildConnection({ modelCount: 0, modelsPreview: [] }))

    expect(
      screen.getByText('No models discovered for this connection yet.')
    ).toBeInTheDocument()
  })

  it('renders the full last-model-sync error text', () => {
    renderSheet(
      buildConnection({
        lastModelSyncError:
          'model discovery failed: upstream returned 503 after 3 retries',
      })
    )

    expect(screen.getByText('Last model sync error')).toBeInTheDocument()
    expect(
      screen.getByText(
        'model discovery failed: upstream returned 503 after 3 retries'
      )
    ).toBeInTheDocument()
  })

  it('hides the sync-error block when the last sync succeeded', () => {
    renderSheet(buildConnection({ lastModelSyncError: null }))

    expect(screen.queryByText('Last model sync error')).not.toBeInTheDocument()
  })

  it('renders the last sync as relative time with an absolute title', () => {
    const twoHoursAgo = new Date(Date.now() - 2 * 60 * 60 * 1000).toISOString()
    renderSheet(buildConnection({ lastModelSyncAt: twoHoursAgo }))

    const relative = screen.getByText('2 hours ago')
    expect(relative).toBeInTheDocument()
    expect(relative.getAttribute('title')).toBeTruthy()
  })

  it('renders the route channel count, including zero', () => {
    renderSheet(buildConnection({ routeChannelCount: 0 }))

    expect(screen.getByText('Route channels')).toBeInTheDocument()
    expect(screen.getByText('0')).toBeInTheDocument()
  })
})

describe('OAuthDetailSheet footer actions', () => {
  it('triggers the page refresh-quota mutation with the rendered connection', () => {
    const onRefreshQuota = vi.fn()
    const connection = buildConnection({})
    renderSheet(connection, { onRefreshQuota })

    fireEvent.click(screen.getByRole('button', { name: 'Refresh quota' }))

    expect(onRefreshQuota).toHaveBeenCalledTimes(1)
    expect(onRefreshQuota).toHaveBeenCalledWith(
      expect.objectContaining({ accountId: 42 })
    )
  })

  it('triggers the page rebind mutation with the rendered connection', () => {
    const onRebind = vi.fn()
    renderSheet(buildConnection({}), { onRebind })

    fireEvent.click(screen.getByRole('button', { name: 'Rebind' }))

    expect(onRebind).toHaveBeenCalledTimes(1)
    expect(onRebind).toHaveBeenCalledWith(
      expect.objectContaining({ accountId: 42 })
    )
  })

  it('disables both actions while the refresh is pending', () => {
    renderSheet(buildConnection({}), { isRefreshingQuota: true })

    // The canonical Spinner (role=status) prepends its localized label to
    // the button's accessible name while pending.
    expect(
      screen.getByRole('button', { name: /Refresh quota/ })
    ).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Rebind' })).toBeDisabled()
  })

  it('disables both actions while the rebind is pending', () => {
    renderSheet(buildConnection({}), { isRebinding: true })

    expect(
      screen.getByRole('button', { name: 'Refresh quota' })
    ).toBeDisabled()
    expect(screen.getByRole('button', { name: /Rebind/ })).toBeDisabled()
  })

  it('warns that rebind reopens the provider authorization flow', () => {
    renderSheet(buildConnection({}))

    expect(
      screen.getByText(
        'Rebind reopens the provider authorization flow in a new tab.'
      )
    ).toBeInTheDocument()
  })
})

describe('OAuthDetailSheet without a connection', () => {
  it('renders an empty panel rather than throwing', () => {
    renderSheet(null)

    expect(screen.queryByText('Overview')).not.toBeInTheDocument()
  })
})
