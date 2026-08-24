// metapi-go/features/token-routes/lib — localStorage-backed preference for
// the routes page's "show zero-channel models" view toggle. Mirrors the
// proxy-logs auto-refresh pattern (the choice is persisted so a bookmarked
// operator view survives reloads and route remounts), scoped to this page
// rather than shared via context: the toggle has exactly one consumer.

import { useCallback, useState } from 'react'

const STORAGE_KEY = 'metapi-go:token-routes:show-zero-channel'

function readStoredPreference(): boolean {
  if (typeof window === 'undefined') return false
  try {
    return window.localStorage.getItem(STORAGE_KEY) === 'true'
  } catch {
    return false
  }
}

function writeStoredPreference(value: boolean) {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(STORAGE_KEY, String(value))
  } catch {
    /* localStorage unavailable — keep the in-memory state only. */
  }
}

/**
 * Resolve the "show zero-channel models" toggle state plus a setter that
 * persists the choice to localStorage. Re-mounts (route navigation,
 * reloads) restore the last choice so the operator's preferred view
 * survives without re-toggling on every visit. Defaults to off — the
 * placeholder rows are an opt-in diagnostic view.
 */
export function useShowZeroChannelPreference(): {
  showZeroChannel: boolean
  setShowZeroChannel: (value: boolean) => void
} {
  const [showZeroChannel, setShowZeroChannelState] =
    useState<boolean>(readStoredPreference)

  const setShowZeroChannel = useCallback((value: boolean) => {
    setShowZeroChannelState(value)
    writeStoredPreference(value)
  }, [])

  return { showZeroChannel, setShowZeroChannel }
}
