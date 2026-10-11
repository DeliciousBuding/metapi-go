import { useEffect, useState } from 'react'

// Refresh only when a displayed cooldown expires, not once per row per second.
export function useUpstreamClock(
  members: readonly { cooldownUntil?: string | null }[]
) {
  const [tick, setTick] = useState(0)
  useEffect(() => {
    const now = Date.now()
    const deadline = members.reduce((next, member) => {
      const value = Date.parse(member.cooldownUntil ?? '')
      return value > now ? Math.min(next, value) : next
    }, Infinity)
    if (!Number.isFinite(deadline)) return
    const timer = window.setTimeout(
      () => setTick((value) => value + 1),
      Math.min(deadline - now + 20, 2_147_483_647)
    )
    return () => window.clearTimeout(timer)
  }, [members, tick])
  return Date.now()
}
