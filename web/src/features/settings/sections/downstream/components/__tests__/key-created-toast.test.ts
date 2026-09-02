// Behavior test for the downstream-key post-create guided toast.
//
// Journey step 4 used to end with a bare "API key created." — the Connect
// dialog auto-opens, but it is dismissible and (#1034) locked behind a
// master-token re-confirm, so closing it left the operator with no way back
// except hunting the row's Connect button in the table. This locks the
// toast's action contract: one success toast, the guided 8s tier, and an
// action that hands the SAME created target back to the host's dialog opener.

import { beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { showKeyCreatedToast } from '../key-created-toast'

const { mockToastSuccess } = vi.hoisted(() => ({
  mockToastSuccess: vi.fn(),
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: mockToastSuccess,
  },
}))

const createdTarget = { id: 7, name: 'Prod key', keyMasked: 'sk-a****c' }

function capturedOptions() {
  expect(mockToastSuccess).toHaveBeenCalledTimes(1)
  return mockToastSuccess.mock.calls[0][1] as {
    description?: string
    duration?: number
    action?: { label?: string; onClick?: () => void }
  }
}

beforeEach(() => {
  mockToastSuccess.mockReset()
})

describe('showKeyCreatedToast', () => {
  it('announces the created key and offers the connect action', () => {
    showKeyCreatedToast(createdTarget, vi.fn())

    expect(mockToastSuccess).toHaveBeenCalledWith(
      'API key created.',
      expect.objectContaining({
        description:
          'Connect a client right away: copy the key or use a one-click import.',
        action: expect.objectContaining({ label: 'Connect now' }),
      })
    )
  })

  it('uses the guided-chain duration tier, not the 3s success default', () => {
    showKeyCreatedToast(createdTarget, vi.fn())

    // Same tier as the account-created and route-completion guided toasts:
    // long enough to read the next step and reach the action.
    expect(capturedOptions().duration).toBe(8000)
  })

  it('hands the same created target back to the host opener', () => {
    const onConnect = vi.fn()

    showKeyCreatedToast(createdTarget, onConnect)
    const action = capturedOptions().action
    expect(action).toBeDefined()
    expect(onConnect).not.toHaveBeenCalled()

    action?.onClick?.()

    expect(onConnect).toHaveBeenCalledTimes(1)
    expect(onConnect).toHaveBeenCalledWith(createdTarget)
  })
})
