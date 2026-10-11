import { cleanup, render, screen, within } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import type { ImportedUpstreamInventory } from '@/lib/api/imported-upstreams'
import type { UpstreamGroup } from '@/lib/api/upstream-catalog'

import { UpstreamRouteTopology } from '../components/upstream-route-topology'

vi.mock('@tanstack/react-router', () => ({
  Link: (props: { children: ReactNode }) => (
    <a href='/token-routes'>{props.children}</a>
  ),
}))
afterEach(cleanup)
const group: UpstreamGroup = {
  id: 1,
  name: 'Work route',
  mode: 'manual',
  enabled: true,
  activeMemberId: 2,
  ownership: 'native',
  routeId: 9,
  modelPattern: 'public-code-model',
  displayName: 'Daily coding',
  members: [
    {
      id: 2,
      groupId: 1,
      grantId: 2,
      priority: 10,
      weight: 1,
      protocolOrder: [],
      ownership: 'native',
      modelName: 'provider-b-model',
      credentialName: 'Spare key',
      channelId: 2,
    },
    {
      id: 1,
      groupId: 1,
      grantId: 1,
      priority: 0,
      weight: 2,
      protocolOrder: [],
      ownership: 'native',
      modelName: 'provider-a-model',
      credentialName: 'Primary key',
      channelId: 1,
    },
  ],
}
const inventory: ImportedUpstreamInventory = {
  items: [
    {
      id: 1,
      name: 'Platform A',
      originKey: 'native',
      dialect: 'generic',
      baseUrl: 'https://example.test',
      enabled: true,
      modelCount: 1,
      credentialCount: 1,
    },
  ],
  members: [
    {
      id: 1,
      groupId: 1,
      groupName: 'Work route',
      routeId: 9,
      channelId: 1,
      modelName: 'provider-a-model',
      credentialName: 'Primary key',
      credentialEnabled: true,
      effectiveEnabled: true,
      protocols: 2,
      mode: 'manual',
      activeItemId: 2,
      priority: 0,
      weight: 2,
    },
  ],
}

describe('upstream routing map', () => {
  it('separates the public model from source models and orders candidates by priority', () => {
    render(
      <UpstreamRouteTopology
        group={group}
        channelId={1}
        inventory={inventory}
        now={0}
      />
    )
    expect(
      screen.getByRole('link', { name: 'public-code-model' })
    ).toBeVisible()
    const rows = within(
      screen.getByRole('list', { name: 'Upstream candidates' })
    ).getAllByRole('listitem')
    expect(within(rows[0]).getByText('provider-a-model')).toBeVisible()
    expect(within(rows[1]).getByText('provider-b-model')).toBeVisible()
    expect(within(rows[0]).getByText('Not eligible')).toBeVisible()
    expect(within(rows[1]).getByText('Manually selected')).toBeVisible()
    expect(within(rows[1]).getByText('Status unavailable')).toBeVisible()
  })
  it('shows cooling and disabled configuration without calling either verified upstream health', () => {
    const data = {
      ...inventory,
      members: [
        { ...inventory.members[0], cooldownUntil: '2026-10-11T01:00:00Z' },
      ],
    }
    const view = render(
      <UpstreamRouteTopology
        group={{ ...group, mode: 'failover' }}
        channelId={1}
        inventory={data}
        now={Date.parse('2026-10-11T00:00:00Z')}
      />
    )
    expect(screen.getByText('Cooling down')).toBeVisible()
    view.rerender(
      <UpstreamRouteTopology
        group={{ ...group, enabled: false }}
        channelId={1}
        inventory={data}
        now={0}
      />
    )
    expect(
      screen.queryByText('Eligible by configuration')
    ).not.toBeInTheDocument()
    expect(screen.getAllByText('Not eligible')).toHaveLength(2)
  })
})
