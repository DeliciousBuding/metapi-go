// metapi-go/features/observability/context — auto-refresh interval state
// shared across the Overview / Health sections. The observability header owns
// a 15s / 30s / Off toggle that drives the `refetchInterval` every query hook
// in `../api.ts` reads, so a single control refreshes all health/heatmap/
// slow-request data instead of each section fetching once on mount.
//
// The provider persists the choice to localStorage so a bookmarked operator
// view survives reloads. The default is 15s (the cadence the audit asks for);
// `false` disables auto-refresh entirely (the queries still honour
// refetchOnWindowFocus=false globally).

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from 'react'

export type ObservabilityAutoRefreshInterval = number | false

const STORAGE_KEY = 'metapi-go:observability:auto-refresh'
const DEFAULT_INTERVAL_MS: ObservabilityAutoRefreshInterval = 15_000

function readStoredInterval(): ObservabilityAutoRefreshInterval {
  if (typeof window === 'undefined') return DEFAULT_INTERVAL_MS
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (raw === null) return DEFAULT_INTERVAL_MS
    if (raw === 'false') return false
    const parsed = Number(raw)
    if (Number.isFinite(parsed) && parsed > 0) return parsed
    return DEFAULT_INTERVAL_MS
  } catch {
    return DEFAULT_INTERVAL_MS
  }
}

function writeStoredInterval(value: ObservabilityAutoRefreshInterval) {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(STORAGE_KEY, String(value))
  } catch {
    /* localStorage unavailable — keep the in-memory state only. */
  }
}

type ObservabilityAutoRefreshContextValue = {
  intervalMs: ObservabilityAutoRefreshInterval
  setIntervalMs: (value: ObservabilityAutoRefreshInterval) => void
}

const ObservabilityAutoRefreshContext =
  createContext<ObservabilityAutoRefreshContextValue | null>(null)

export function ObservabilityAutoRefreshProvider({
  children,
}: {
  children: ReactNode
}) {
  const [intervalMs, setIntervalMsState] = useState<
    ObservabilityAutoRefreshInterval
  >(readStoredInterval)

  const setIntervalMs = useCallback(
    (value: ObservabilityAutoRefreshInterval) => {
      setIntervalMsState(value)
      writeStoredInterval(value)
    },
    []
  )

  const value = useMemo(
    () => ({ intervalMs, setIntervalMs }),
    [intervalMs, setIntervalMs]
  )

  return (
    <ObservabilityAutoRefreshContext.Provider value={value}>
      {children}
    </ObservabilityAutoRefreshContext.Provider>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export function useObservabilityAutoRefresh(): ObservabilityAutoRefreshContextValue {
  const ctx = useContext(ObservabilityAutoRefreshContext)
  if (!ctx) {
    throw new Error(
      'useObservabilityAutoRefresh must be used within an ObservabilityAutoRefreshProvider'
    )
  }
  return ctx
}
