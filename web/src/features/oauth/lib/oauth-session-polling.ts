// metapi-go/features/oauth — bounded polling for a pending OAuth session.
//
// After `startOAuthProvider` returns a `state`, the backend settles the
// session asynchronously once the OAuth callback lands
// (GET /api/oauth/sessions/:state → status pending|success|error). This hook
// polls that endpoint with a bounded budget: the first check fires
// immediately, then every `OAUTH_SESSION_POLL_INTERVAL_MS` until the session
// settles or `OAUTH_SESSION_POLL_MAX_ATTEMPTS` runs out. Exhaustion is
// surfaced honestly (`exhausted: true`) so the UI can say "still waiting —
// paste the callback manually" instead of pretending success. A transient
// fetch failure consumes one attempt but does not settle the session.
//
// Cleanup: clearing `state` (dialog closed / user cancelled) or unmounting
// cancels the scheduled timer and ignores any in-flight response.
//
// `kick()` restarts polling immediately with a fresh budget — used after a
// manual-callback submission, which settles (or advances) the session on the
// backend without waiting out the next interval tick.

import { useCallback, useEffect, useState } from 'react'

import { api, type OAuthSessionInfo } from '@/lib/api'

/** Delay between session checks while the session is still pending. */
export const OAUTH_SESSION_POLL_INTERVAL_MS = 2000

/** How many session checks to run before pausing and asking for a manual callback. */
export const OAUTH_SESSION_POLL_MAX_ATTEMPTS = 30

export type OAuthSessionPolling = {
  /** Latest polled session snapshot; `null` before the first check resolves. */
  session: OAuthSessionInfo | null
  /** 1-based index of the check that is running / ran last. */
  attempt: number
  /** True once the budget ran out while the session was still pending. */
  exhausted: boolean
  /** Restart polling immediately with a fresh budget. */
  kick: () => void
}

/**
 * Poll a pending OAuth session by `state` until the backend settles it or
 * the attempt budget runs out. Pass `null` to disable polling entirely
 * (e.g. before the start-authorization mutation has returned a `state`).
 */
export function useOAuthSessionPolling(
  state: string | null
): OAuthSessionPolling {
  const [session, setSession] = useState<OAuthSessionInfo | null>(null)
  const [attempt, setAttempt] = useState(0)
  const [exhausted, setExhausted] = useState(false)
  const [kickToken, setKickToken] = useState(0)

  useEffect(() => {
    if (!state) return
    let cancelled = false
    let timer: ReturnType<typeof setTimeout> | undefined

    // Fresh wait window: drop the previous snapshot before the first check.
    setSession(null)
    setAttempt(0)
    setExhausted(false)

    const runCheck = (attemptNumber: number) => {
      if (cancelled) return
      setAttempt(attemptNumber)

      const continuePolling = () => {
        if (attemptNumber >= OAUTH_SESSION_POLL_MAX_ATTEMPTS) {
          setExhausted(true)
          return
        }
        timer = setTimeout(
          () => runCheck(attemptNumber + 1),
          OAUTH_SESSION_POLL_INTERVAL_MS
        )
      }

      api
        .getOAuthSession(state)
        .then((info) => {
          if (cancelled) return
          setSession(info)
          if (info.status !== 'pending') return
          continuePolling()
        })
        .catch(() => {
          // Transient failure: consumes the attempt, does not settle the
          // session — keep polling until the budget runs out.
          if (cancelled) return
          continuePolling()
        })
    }

    runCheck(1)

    return () => {
      cancelled = true
      if (timer !== undefined) clearTimeout(timer)
    }
  }, [state, kickToken])

  const kick = useCallback(() => setKickToken((token) => token + 1), [])

  return { session, attempt, exhausted, kick }
}
