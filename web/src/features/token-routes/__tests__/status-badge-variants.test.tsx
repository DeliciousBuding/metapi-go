// Badge recipe convergence regression (audit P1 #1): route list badges use
// semantic soft variants — the enabled state uses success (not the solid
// `default` primary block) and the enabled/total channel summary ladder is
// success (all enabled) / warning (partial) / secondary (none enabled).

import '@testing-library/jest-dom/vitest'
import { cleanup, render } from '@testing-library/react'
import type { ReactElement } from 'react'
import { afterEach, describe, expect, it } from 'vitest'

import '@/i18n/config'

import { useRoutesColumns } from '../components/routes-columns'
import type { RouteRowActions, RouteSummaryRow } from '../types'

const noopActions: RouteRowActions = {
  onEdit: () => {},
  onDelete: () => {},
  onToggleEnabled: () => {},
  onViewDetail: () => {},
  onClearCooldown: () => {},
  onRefreshDecision: () => {},
}

function makeRoute(overrides: Partial<RouteSummaryRow>): RouteSummaryRow {
  return {
    id: 1,
    modelPattern: 'gpt-*',
    displayName: null,
    displayIcon: null,
    modelMapping: null,
    enabled: true,
    channelCount: 3,
    enabledChannelCount: 3,
    siteNames: [],
    decisionSnapshot: null,
    decisionRefreshedAt: null,
    ...overrides,
  }
}

function RouteCell({
  columnId,
  route,
}: {
  columnId: string
  route: RouteSummaryRow
}) {
  const columns = useRoutesColumns(noopActions)
  const column = columns.find((entry) => entry.id === columnId)
  if (!column?.cell) throw new Error(`${columnId} column cell missing`)
  const cell = column.cell as unknown as (context: {
    row: { original: RouteSummaryRow }
  }) => ReactElement
  return cell({ row: { original: route } })
}

function readBadgeVariant(container: HTMLElement): string | null {
  const badge = container.querySelector('[data-slot="badge"]')
  return badge?.getAttribute('data-variant') ?? null
}

afterEach(() => cleanup())

describe('route enabled badge variants', () => {
  it('renders an enabled route with the success soft variant', () => {
    const { container } = render(
      <RouteCell columnId='enabled' route={makeRoute({ enabled: true })} />
    )
    expect(readBadgeVariant(container)).toBe('success')
  })

  it('renders a disabled route with the neutral secondary variant', () => {
    const { container } = render(
      <RouteCell columnId='enabled' route={makeRoute({ enabled: false })} />
    )
    expect(readBadgeVariant(container)).toBe('secondary')
  })
})

describe('route channel summary badge variants', () => {
  it('renders an all-enabled channel count with the success variant', () => {
    const { container } = render(
      <RouteCell
        columnId='channels'
        route={makeRoute({ channelCount: 3, enabledChannelCount: 3 })}
      />
    )
    expect(readBadgeVariant(container)).toBe('success')
  })

  it('renders a partially disabled channel count with the warning variant', () => {
    const { container } = render(
      <RouteCell
        columnId='channels'
        route={makeRoute({ channelCount: 3, enabledChannelCount: 1 })}
      />
    )
    expect(readBadgeVariant(container)).toBe('warning')
  })

  it('renders an all-disabled channel count with the secondary variant', () => {
    const { container } = render(
      <RouteCell
        columnId='channels'
        route={makeRoute({ channelCount: 2, enabledChannelCount: 0 })}
      />
    )
    expect(readBadgeVariant(container)).toBe('secondary')
  })
})
