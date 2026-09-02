// Behavior test for the route-completion guided toast. Locks the SPA
// navigation contract: the toast action must route through the shared
// router instance (no window.location hard reload) and land on the
// first-class /downstream-keys page — the step 3 → step 4 handoff of the
// site → account → route → key journey. It used to point at the pre-promotion
// /settings/downstream subarea; the left nav links /downstream-keys, so the
// guided chain now lands where the operator will look for it afterwards.

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
  it('routes the toast action to Downstream Keys via the SPA router', async () => {
    showRouteCompletionToast(11, { accountId: 42, siteId: 7 })

    expect(mockToastSuccess).toHaveBeenCalledTimes(1)
    // The action label names the actual next step (issuing a key), not the
    // vaguer "connect a client" it carried while it pointed at Settings.
    const label = (
      mockToastSuccess.mock.calls[0][1] as {
        action?: { label?: string }
      }
    ).action?.label
    expect(label).toBe('Issue a downstream key')
    const options = mockToastSuccess.mock.calls[0][1] as {
      action?: { onClick?: () => Promise<void> }
    }
    const action = options.action as { onClick: () => Promise<void> }
    await action.onClick()

    expect(mockNavigate).toHaveBeenCalledWith({ to: '/downstream-keys' })
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
