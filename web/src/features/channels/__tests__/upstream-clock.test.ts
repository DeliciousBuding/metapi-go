import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'

import { useUpstreamClock } from '../hooks/use-upstream-clock'

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

it('refreshes at cooldown expiry without polling every member', () => {
  vi.useFakeTimers()
  vi.setSystemTime(new Date('2026-10-11T00:00:00Z'))
  const members = [
    { cooldownUntil: '2026-10-11T00:00:01Z' },
    { cooldownUntil: '2026-10-11T00:00:02Z' },
  ]
  const { result, unmount } = renderHook(() => useUpstreamClock(members))
  expect(vi.getTimerCount()).toBe(1)
  act(() => vi.advanceTimersByTime(1020))
  expect(result.current).toBe(Date.parse('2026-10-11T00:00:01.020Z'))
  expect(vi.getTimerCount()).toBe(1)
  unmount()
  expect(vi.getTimerCount()).toBe(0)
})
