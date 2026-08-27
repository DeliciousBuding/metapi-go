// Behavior tests for the account-created guided toast. Lock the SPA
// navigation contract: the toast action must route through the shared
// router instance (no window.location hard reload) and keep the
// /token-routes deep link with the new account/site preselected.
//
// The four post-create token sync states (#1002) are locked too: synced
// reports the real count, empty warns that nothing was found, failed
// downgrades to a partial-initialization warning, and skipped/absent
// reports keep the original guided copy. Every state keeps the CTA.

import { beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import {
  showAccountCreatedToast,
  showAccountLoginToast,
} from './account-created-toast'

const { mockNavigate, mockToastSuccess, mockToastWarning } = vi.hoisted(() => ({
  mockNavigate: vi.fn(),
  mockToastSuccess: vi.fn(),
  mockToastWarning: vi.fn(),
}))

vi.mock('@/lib/router', () => ({
  router: { navigate: mockNavigate },
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: mockToastSuccess,
    warning: mockToastWarning,
  },
}))

type ToastOptions = {
  description?: string
  duration?: number
  action?: { onClick?: () => Promise<void> }
}

function captureToastAction(mock: ReturnType<typeof vi.fn>): {
  onClick: () => Promise<void>
} {
  expect(mock).toHaveBeenCalledTimes(1)
  const options = mock.mock.calls[0][1] as ToastOptions
  expect(options.action?.onClick).toBeTypeOf('function')
  return options.action as { onClick: () => Promise<void> }
}

function toastOptions(mock: ReturnType<typeof vi.fn>): ToastOptions {
  expect(mock).toHaveBeenCalledTimes(1)
  return mock.mock.calls[0][1] as ToastOptions
}

async function expectRoutesCta(mock: ReturnType<typeof vi.fn>) {
  const action = captureToastAction(mock)
  await action.onClick()
  expect(mockNavigate).toHaveBeenCalledWith({
    to: '/token-routes',
    search: { accountId: 42, siteId: 7 },
  })
}

beforeEach(() => {
  mockNavigate.mockReset()
  mockToastSuccess.mockReset()
  mockToastWarning.mockReset()
})

describe('showAccountCreatedToast', () => {
  it('routes the toast action to /token-routes via the SPA router', async () => {
    showAccountCreatedToast(42, 7)

    await expectRoutesCta(mockToastSuccess)
  })

  it('navigates without preselection when the ids are unknown', async () => {
    showAccountCreatedToast()

    const action = captureToastAction(mockToastSuccess)
    await action.onClick()

    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/token-routes',
      search: { accountId: undefined, siteId: undefined },
    })
  })

  it('reports the real synced token count with the routes CTA', async () => {
    showAccountCreatedToast(42, 7, {
      tokenCount: 3,
      tokenSyncStatus: 'synced',
      tokenSyncMessage: 'synced 3 tokens',
    })

    const options = toastOptions(mockToastSuccess)
    expect(mockToastWarning).not.toHaveBeenCalled()
    expect(options.description).toContain('3')
    await expectRoutesCta(mockToastSuccess)
  })

  it('reports empty upstream sync as a success with a hint', async () => {
    showAccountCreatedToast(42, 7, {
      tokenCount: 0,
      tokenSyncStatus: 'empty',
      tokenSyncMessage: 'no upstream tokens',
    })

    expect(mockToastSuccess).toHaveBeenCalledTimes(1)
    expect(mockToastWarning).not.toHaveBeenCalled()
    await expectRoutesCta(mockToastSuccess)
  })

  it('downgrades a failed sync to a partial-initialization warning', async () => {
    showAccountCreatedToast(42, 7, {
      tokenCount: 0,
      tokenSyncStatus: 'failed',
      tokenSyncMessage:
        'partial initialization: token sync failed: upstream 500',
    })

    expect(mockToastSuccess).not.toHaveBeenCalled()
    const options = toastOptions(mockToastWarning)
    expect(options.duration).toBe(8000)
    expect(options.description).toContain('upstream 500')
    await expectRoutesCta(mockToastWarning)
  })

  it('keeps the original copy for skipped syncs (API-key connections)', async () => {
    showAccountCreatedToast(42, 7, {
      tokenCount: 0,
      tokenSyncStatus: 'skipped',
      tokenSyncMessage: 'API key connection; token sync skipped',
    })

    expect(mockToastSuccess).toHaveBeenCalledTimes(1)
    expect(mockToastWarning).not.toHaveBeenCalled()
    await expectRoutesCta(mockToastSuccess)
  })
})

describe('showAccountLoginToast', () => {
  it('keeps the compact success toast for legacy or skipped sync results', () => {
    showAccountLoginToast(42, 7, false, {
      tokenCount: 0,
      tokenSyncStatus: 'skipped',
      tokenSyncMessage: 'API key connection; token sync skipped',
    })

    expect(mockToastSuccess).toHaveBeenCalledTimes(1)
    expect(mockToastSuccess.mock.calls[0]?.[1]).toBeUndefined()
    expect(mockToastWarning).not.toHaveBeenCalled()
  })

  it('shows synced token count and keeps the route CTA after binding', async () => {
    showAccountLoginToast(42, 7, true, {
      tokenCount: 2,
      tokenSyncStatus: 'synced',
      tokenSyncMessage: 'synced 2 tokens',
    })

    const options = toastOptions(mockToastSuccess)
    expect(mockToastSuccess.mock.calls[0]?.[0]).toBe(
      'Account session refreshed'
    )
    expect(options.description).toContain('2')
    await expectRoutesCta(mockToastSuccess)
  })

  it('shows an empty sync hint after binding without inventing tokens', () => {
    showAccountLoginToast(42, 7, false, {
      tokenCount: 0,
      tokenSyncStatus: 'empty',
      tokenSyncMessage: 'no upstream tokens',
    })

    const options = toastOptions(mockToastSuccess)
    expect(mockToastSuccess.mock.calls[0]?.[0]).toBe(
      'Account bound successfully'
    )
    expect(options.description).toContain('No upstream tokens')
  })

  it('warns on failed binding sync and keeps the route CTA', async () => {
    showAccountLoginToast(42, 7, false, {
      tokenCount: 0,
      tokenSyncStatus: 'failed',
      tokenSyncMessage:
        'partial initialization: token sync failed: upstream 500',
    })

    expect(mockToastSuccess).not.toHaveBeenCalled()
    const options = toastOptions(mockToastWarning)
    expect(mockToastWarning.mock.calls[0]?.[0]).toBe(
      'Account bound, but token sync failed'
    )
    expect(options.description).toContain('upstream 500')
    await expectRoutesCta(mockToastWarning)
  })
})
