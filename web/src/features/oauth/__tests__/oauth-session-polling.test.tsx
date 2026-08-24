// Bounded-polling behavior for pending OAuth sessions (fake timers).
//
// `useOAuthSessionPolling` polls `api.getOAuthSession(state)` with a bounded
// attempt budget: immediate first check, then every
// OAUTH_SESSION_POLL_INTERVAL_MS until the session settles or
// OAUTH_SESSION_POLL_MAX_ATTEMPTS runs out. These tests verify the success
// path, the honest exhaustion path (no fake success), transient-error
// tolerance, cleanup on unmount / state change, and the manual `kick()`.

import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  OAUTH_SESSION_POLL_INTERVAL_MS,
  OAUTH_SESSION_POLL_MAX_ATTEMPTS,
  useOAuthSessionPolling,
} from '../lib/oauth-session-polling'

const TEST_STATE = 'test-session-state'

const mockGetSession = vi.hoisted(() => vi.fn())

vi.mock('@/lib/api', () => ({
  api: { getOAuthSession: mockGetSession },
}))

function pendingSession() {
  return { provider: 'openai', state: TEST_STATE, status: 'pending' as const }
}

function successSession() {
  return {
    provider: 'openai',
    state: TEST_STATE,
    status: 'success' as const,
    accountId: 7,
  }
}

/** Flush pending microtasks (resolved fetch promises + state updates). */
async function settle() {
  await act(async () => {
    await Promise.resolve()
  })
}

/** Advance the fake clock by one poll interval, flushing microtasks too. */
async function advanceOneInterval() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(OAUTH_SESSION_POLL_INTERVAL_MS)
  })
}

beforeEach(() => {
  vi.useFakeTimers()
  mockGetSession.mockReset()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('useOAuthSessionPolling', () => {
  it('does nothing while state is null', async () => {
    const { result } = renderHook(() => useOAuthSessionPolling(null))

    await settle()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000)
    })

    expect(mockGetSession).not.toHaveBeenCalled()
    expect(result.current.session).toBeNull()
    expect(result.current.exhausted).toBe(false)
  })

  it('polls until the session settles, then stops', async () => {
    mockGetSession
      .mockResolvedValueOnce(pendingSession())
      .mockResolvedValueOnce(pendingSession())
      .mockResolvedValueOnce(successSession())

    const { result } = renderHook(() => useOAuthSessionPolling(TEST_STATE))

    // The first check fires immediately when the state is set.
    await settle()
    expect(mockGetSession).toHaveBeenCalledTimes(1)
    expect(mockGetSession).toHaveBeenCalledWith(TEST_STATE)
    expect(result.current.session?.status).toBe('pending')
    expect(result.current.attempt).toBe(1)

    await advanceOneInterval() // attempt 2 → pending
    expect(mockGetSession).toHaveBeenCalledTimes(2)
    await advanceOneInterval() // attempt 3 → success
    expect(mockGetSession).toHaveBeenCalledTimes(3)
    expect(result.current.session?.status).toBe('success')
    expect(result.current.exhausted).toBe(false)

    // Settled: no further checks even after a long wait.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000)
    })
    expect(mockGetSession).toHaveBeenCalledTimes(3)
  })

  it('exhausts the budget honestly instead of pretending success', async () => {
    mockGetSession.mockImplementation(() => Promise.resolve(pendingSession()))

    const { result } = renderHook(() => useOAuthSessionPolling(TEST_STATE))

    await settle()
    for (let i = 1; i < OAUTH_SESSION_POLL_MAX_ATTEMPTS; i += 1) {
      await advanceOneInterval()
    }

    expect(mockGetSession).toHaveBeenCalledTimes(
      OAUTH_SESSION_POLL_MAX_ATTEMPTS
    )
    expect(result.current.exhausted).toBe(true)
    // The last snapshot is still pending — exhaustion is NOT success.
    expect(result.current.session?.status).toBe('pending')

    // The budget is final: waiting longer produces no extra checks.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10 * OAUTH_SESSION_POLL_INTERVAL_MS)
    })
    expect(mockGetSession).toHaveBeenCalledTimes(
      OAUTH_SESSION_POLL_MAX_ATTEMPTS
    )
  })

  it('stops polling after unmount', async () => {
    mockGetSession.mockImplementation(() => Promise.resolve(pendingSession()))

    const { unmount } = renderHook(() => useOAuthSessionPolling(TEST_STATE))

    await settle()
    await advanceOneInterval()
    expect(mockGetSession).toHaveBeenCalledTimes(2)

    unmount()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(60_000)
    })
    expect(mockGetSession).toHaveBeenCalledTimes(2)
  })

  it('clears the previous timer and restarts when the state changes', async () => {
    mockGetSession.mockImplementation(() => Promise.resolve(pendingSession()))

    const { result, rerender } = renderHook(
      ({ state }: { state: string | null }) => useOAuthSessionPolling(state),
      { initialProps: { state: TEST_STATE } }
    )

    await settle()
    await advanceOneInterval()
    expect(mockGetSession).toHaveBeenCalledTimes(2)

    rerender({ state: 'other-state' })

    // The new state fires an immediate check…
    await settle()
    expect(mockGetSession).toHaveBeenCalledTimes(3)
    expect(mockGetSession).toHaveBeenLastCalledWith('other-state')
    expect(result.current.attempt).toBe(1)

    // …and one interval later exactly one more check (the old timer for
    // TEST_STATE was cleared, not stacked on top of the new one).
    await advanceOneInterval()
    expect(mockGetSession).toHaveBeenCalledTimes(4)
    expect(mockGetSession).toHaveBeenLastCalledWith('other-state')
  })

  it('keeps polling after a transient fetch failure (attempt consumed)', async () => {
    mockGetSession
      .mockRejectedValueOnce(new Error('network down'))
      .mockImplementation(() => Promise.resolve(pendingSession()))

    const { result } = renderHook(() => useOAuthSessionPolling(TEST_STATE))

    await settle()
    expect(mockGetSession).toHaveBeenCalledTimes(1)
    // The failure kept the previous snapshot (none yet).
    expect(result.current.session).toBeNull()
    expect(result.current.exhausted).toBe(false)

    await advanceOneInterval()
    expect(mockGetSession).toHaveBeenCalledTimes(2)
    expect(result.current.session?.status).toBe('pending')
  })

  it('kick() restarts polling immediately with a fresh budget', async () => {
    mockGetSession.mockImplementation(() => Promise.resolve(pendingSession()))

    const { result } = renderHook(() => useOAuthSessionPolling(TEST_STATE))

    await settle()
    await advanceOneInterval()
    expect(mockGetSession).toHaveBeenCalledTimes(2)
    expect(result.current.attempt).toBe(2)

    act(() => {
      result.current.kick()
    })
    // No timer advance needed — the restart fires an immediate check.
    await settle()

    expect(mockGetSession).toHaveBeenCalledTimes(3)
    expect(result.current.attempt).toBe(1)
    expect(result.current.exhausted).toBe(false)
  })
})
