// Behavior test for the route-completion guided toast. Locks the SPA
// navigation contract: the toast action must route through the shared
// router instance (no window.location hard reload) and land on the
// Settings → Downstream subarea, same URL as before the fix.

import { beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { showRouteCompletionToast } from './route-completion-toast'

const { mockNavigate, mockToastSuccess } = vi.hoisted(() => ({
  mockNavigate: vi.fn(),
  mockToastSuccess: vi.fn(),
}))

vi.mock('@/lib/router', () => ({
  router: { navigate: mockNavigate },
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: mockToastSuccess,
  },
}))

beforeEach(() => {
  mockNavigate.mockReset()
  mockToastSuccess.mockReset()
})

describe('showRouteCompletionToast', () => {
  it('routes the toast action to Settings → Downstream via the SPA router', async () => {
    showRouteCompletionToast(11, { accountId: 42, siteId: 7 })

    expect(mockToastSuccess).toHaveBeenCalledTimes(1)
    const options = mockToastSuccess.mock.calls[0][1] as {
      action?: { onClick?: () => Promise<void> }
    }
    const action = options.action as { onClick: () => Promise<void> }
    await action.onClick()

    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/settings/$subarea',
      params: { subarea: 'downstream' },
    })
  })

  it('still navigates when fired without route context', async () => {
    showRouteCompletionToast()

    const options = mockToastSuccess.mock.calls[0][1] as {
      action?: { onClick?: () => Promise<void> }
    }
    const action = options.action as { onClick: () => Promise<void> }
    await action.onClick()

    expect(mockNavigate).toHaveBeenCalledTimes(1)
  })
})
