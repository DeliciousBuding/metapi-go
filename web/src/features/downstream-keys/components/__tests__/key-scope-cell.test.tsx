import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactElement } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { KeyScopeCell, type ScopeNameMaps } from '../key-scope-cell'

afterEach(() => cleanup())

const names: ScopeNameMaps = {
  sites: new Map([
    [1, 'Alpha'],
    [2, 'Beta'],
  ]),
  accounts: new Map([
    [11, 'alice'],
    [12, 'bob'],
  ]),
  tokens: new Map([[101, 'Token A']]),
}

function renderCell(
  item: Parameters<typeof KeyScopeCell>[0]['item'],
  onOpenDetails?: (anchorField: string | null) => void
) {
  return render(
    (
      <KeyScopeCell item={item} names={names} onOpenDetails={onOpenDetails} />
    ) as ReactElement
  )
}

describe('KeyScopeCell restriction badges (#1430)', () => {
  it('renders a single Unrestricted badge when nothing is configured', () => {
    renderCell({})
    expect(screen.getByText('Unrestricted')).toBeInTheDocument()
    expect(screen.queryByTestId('key-scope-cell')).toBeNull()
  })

  it('renders one badge per configured dimension with its count', () => {
    renderCell({
      supportedModels: ['gpt-5.5', 'claude-*'],
      allowedSiteIds: [1, 2],
      maxRpm: 60,
      maxCost: 50,
    })
    expect(screen.getByText('Models · 2')).toBeInTheDocument()
    expect(screen.getByText('Sites · 2')).toBeInTheDocument()
    expect(screen.getByText('Rate · 1')).toBeInTheDocument()
    expect(screen.getByText('Budget · 1')).toBeInTheDocument()
  })

  it('carries the human-readable detail in the badge tooltip', () => {
    renderCell({
      allowedSiteIds: '[1]',
      excludedCredentialRefs: JSON.stringify([
        { kind: 'default_api_key', siteId: 2, accountId: 12 },
      ]),
      ipAllowlist: '10.0.0.0/8',
    })
    expect(screen.getByText('Sites · 1')).toHaveAttribute(
      'title',
      'Allowed sites (1): Alpha'
    )
    expect(screen.getByText('Credentials · 1')).toHaveAttribute(
      'title',
      'Excluded credentials (1): Beta / bob / default key'
    )
    expect(screen.getByText('IP lists · 1')).toHaveAttribute(
      'title',
      'Allow: 10.0.0.0/8'
    )
  })

  it('resolves missing mappings as unresolved IDs in the tooltip', () => {
    renderCell({
      allowedCredentialRefs: JSON.stringify([
        { kind: 'account_token', siteId: 7, accountId: 8, tokenId: 9 },
      ]),
    })
    expect(screen.getByText('Credentials · 1')).toHaveAttribute(
      'title',
      'Allowed credentials (1): ID 7 (unresolved) / ID 8 (unresolved) / ID 9 (unresolved)'
    )
  })

  it('keeps the access-policy honesty markers in the tooltip', () => {
    // Present-but-empty direct allow-list: every channel denied — a fact the
    // operator must see, not a rounded-off "restricted".
    const denied = renderCell({
      accessPolicy: { allowedUpstreamChannelIds: [] },
    })
    const deniedBadge = screen.getByText(/^Policy · /)
    expect(deniedBadge).toHaveAttribute(
      'title',
      expect.stringContaining('All channels denied')
    )
    denied.unmount()

    cleanup()
    renderCell({ accessPolicy: 'not-json' })
    expect(screen.getByText('Policy · Unknown')).toHaveAttribute(
      'title',
      'Policy unreadable — restrictions preserved'
    )
  })

  it('lists the access policy content in the tooltip', () => {
    renderCell({
      accessPolicy: JSON.stringify({
        allowedUpstreamChannelIds: [2, 5],
        modelIds: ['coding'],
        modelMappings: [{ from: 'client', to: 'coding' }],
        quota: {
          requests: 0,
          period: { type: 'all_time' },
          historyMissingBefore: 0,
        },
      }),
    })
    const badge = screen.getByText(/^Policy · /)
    const title = badge.getAttribute('title') ?? ''
    expect(title).toContain('#2, #5')
    expect(title).toContain('coding')
    expect(title).toContain('Model mappings')
    expect(title).toContain('Request count: 0')
    expect(title).toContain('Quota history incomplete')
  })

  it('clicking a badge reports the dimension anchor of its form section', () => {
    const onOpenDetails = vi.fn()
    renderCell(
      {
        supportedModels: ['gpt-5.5'],
        allowedSiteIds: [1],
        allowedRouteIds: [9],
        ipBlocklist: '10.0.0.1',
      },
      onOpenDetails
    )
    fireEvent.click(screen.getByText('Models · 1'))
    expect(onOpenDetails).toHaveBeenLastCalledWith('supportedModels')
    fireEvent.click(screen.getByText('Sites · 1'))
    expect(onOpenDetails).toHaveBeenLastCalledWith('allowedSiteIds')
    // routes and ip have no form editing surface — they open the sheet
    // without a locate target.
    fireEvent.click(screen.getByText('Routes · 1'))
    expect(onOpenDetails).toHaveBeenLastCalledWith(null)
    fireEvent.click(screen.getByText('IP lists · 1'))
    expect(onOpenDetails).toHaveBeenLastCalledWith(null)
  })

  it('renders non-interactive badges when no click-through handler is given', () => {
    renderCell({ supportedModels: ['gpt-5.5'] })
    expect(screen.getByText('Models · 1').tagName).toBe('SPAN')
  })
})
