// Behavior test for the account-created guided toast. Locks the SPA
// navigation contract: the toast action must route through the shared
// router instance (no window.location hard reload) and keep the
// /token-routes deep link with the new account/site preselected.

import { beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { showAccountCreatedToast } from './account-created-toast'

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

function captureToastAction(): { onClick: () => Promise<void> } {
  expect(mockToastSuccess).toHaveBeenCalledTimes(1)
  const options = mockToastSuccess.mock.calls[0][1] as {
    action?: { onClick?: () => Promise<void> }
  }
  expect(options.action?.onClick).toBeTypeOf('function')
  return options.action as { onClick: () => Promise<void> }
}

beforeEach(() => {
  mockNavigate.mockReset()
  mockToastSuccess.mockReset()
})

describe('showAccountCreatedToast', () => {
  it('routes the toast action to /token-routes via the SPA router', async () => {
    showAccountCreatedToast(42, 7)

    const action = captureToastAction()
    await action.onClick()

    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/token-routes',
      search: { accountId: 42, siteId: 7 },
    })
  })

  it('navigates without preselection when the ids are unknown', async () => {
    showAccountCreatedToast()

    const action = captureToastAction()
    await action.onClick()

    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/token-routes',
      search: { accountId: undefined, siteId: undefined },
    })
  })
})
