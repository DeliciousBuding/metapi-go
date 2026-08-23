// Regression tests for the route-channel editor embedded in the edit dialog
// (Wave 7 L6: per-channel weight/priority/enabled/delete was previously
// uneditable in the UI — backend PUT/DELETE /api/channels had zero consumers).
// Covers: existing channels are listed with their fetched values, a weight
// commit PUTs, the enabled switch PUTs, and delete requires confirmation.
import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/i18n/config'

import { RouteChannelEditor } from '../route-channel-editor'
import type { RouteChannel } from '../../types'

const testState = vi.hoisted(() => ({
  channels: [
    {
      id: 1,
      routeId: 1,
      accountId: 10,
      tokenId: null,
      sourceModel: 'gpt-4o',
      priority: 0,
      weight: 10,
      enabled: true,
      manualOverride: false,
      successCount: 0,
      failCount: 0,
      cooldownUntil: null,
      account: { username: 'alice' },
      site: { id: 1, name: 'NewAPI 公益站', platform: 'newapi' },
      token: null,
    },
    {
      id: 2,
      routeId: 1,
      accountId: 11,
      tokenId: null,
      sourceModel: 'gpt-4o',
      priority: 1,
      weight: 4,
      enabled: false,
      manualOverride: true,
      successCount: 0,
      failCount: 0,
      cooldownUntil: null,
      account: { username: 'bob' },
      site: { id: 2, name: 'OneAPI 聚合', platform: 'oneapi' },
      token: null,
    },
  ] as RouteChannel[],
  updateMutate: vi.fn(),
  deleteMutate: vi.fn(),
}))

vi.mock('../../api', () => ({
  useRouteChannels: () => ({
    data: testState.channels,
    isLoading: false,
    isFetching: false,
  }),
  useUpdateChannel: () => ({
    mutate: testState.updateMutate,
    variables: undefined,
    isPending: false,
  }),
  useDeleteChannel: () => ({
    mutate: testState.deleteMutate,
    variables: undefined,
    isPending: false,
  }),
}))

beforeAll(async () => {
  // Tests assert zh-CN copy directly (same strings the UI shows).
  await i18n.changeLanguage('zhCN')
})
beforeEach(() => {
  testState.updateMutate.mockClear()
  testState.deleteMutate.mockClear()
})
afterEach(cleanup)

describe('RouteChannelEditor', () => {
  it('lists the route channels with weight/priority inputs and switches', () => {
    render(<RouteChannelEditor routeId={1} />)
    expect(screen.getByText('路由通道')).toBeInTheDocument()
    const rows = screen.getAllByRole('listitem')
    expect(rows).toHaveLength(2)
    const first = within(rows[0])
    const [weightInput, priorityInput] = first.getAllByRole('spinbutton')
    expect(weightInput).toHaveValue(10)
    expect(priorityInput).toHaveValue(0)
    expect(first.getByText('alice')).toBeInTheDocument()
    const switches = first.getAllByRole('switch')
    expect(switches[0]).toHaveAttribute('aria-checked', 'true')
    expect(within(rows[1]).getAllByRole('switch')[0]).toHaveAttribute(
      'aria-checked',
      'false'
    )
  })

  it('commits a weight change via blur/Enter to the update mutation', () => {
    render(<RouteChannelEditor routeId={1} />)
    const rows = screen.getAllByRole('listitem')
    const weightInput = within(rows[0]).getAllByRole('spinbutton')[0]
    fireEvent.change(weightInput, { target: { value: '12' } })
    fireEvent.keyDown(weightInput, { key: 'Enter' })
    fireEvent.blur(weightInput)
    expect(testState.updateMutate.mock.calls[0]?.[0]).toEqual({
      id: 1,
      data: { weight: 12 },
    })
  })

  it('reverts a non-integer draft instead of committing it', () => {
    render(<RouteChannelEditor routeId={1} />)
    const rows = screen.getAllByRole('listitem')
    const weightInput = within(rows[0]).getAllByRole('spinbutton')[0]
    fireEvent.change(weightInput, { target: { value: '1.5' } })
    fireEvent.blur(weightInput)
    expect(testState.updateMutate).not.toHaveBeenCalled()
    expect(weightInput).toHaveValue(10)
  })

  it('PUTs the enabled toggle', () => {
    render(<RouteChannelEditor routeId={1} />)
    const rows = screen.getAllByRole('listitem')
    const switchEl = within(rows[1]).getAllByRole('switch')[0]
    fireEvent.click(switchEl)
    expect(testState.updateMutate.mock.calls[0]?.[0]).toEqual({
      id: 2,
      data: { enabled: true },
    })
  })

  it('removes a channel only after confirm', () => {
    render(<RouteChannelEditor routeId={1} />)
    const rows = screen.getAllByRole('listitem')
    fireEvent.click(
      within(rows[1]).getByRole('button', { name: '移除通道' })
    )
    expect(testState.deleteMutate).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: '移除' }))
    expect(testState.deleteMutate.mock.calls[0]?.[0]).toEqual(2)
  })
})
