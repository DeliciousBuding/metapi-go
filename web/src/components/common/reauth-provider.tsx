// metapi-go/components/common — master-token re-confirmation host (#1034).
//
// The backend rejects sensitive admin operations with
// `403 { reauthRequired: true }` until the request carries the master token
// in X-Admin-Confirm-Token. Features call `requestReauth` when they see that
// rejection; the host renders the shared ReauthDialog once and hands the
// entered token straight back to the replay callback.
//
// The token never leaves the dialog: it is not stored in context, the query
// cache, localStorage or the URL. The host only remembers which operation is
// waiting to be replayed.

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { useTranslation } from 'react-i18next'

import { isReauthRequired } from '@/lib/http-client'

import { ReauthDialog } from './reauth-dialog'

export type ReauthRequest = {
  /** What the pending operation does, shown above the token input. */
  description: string
  /**
   * Replays the rejected request with the entered master token. Resolve once
   * the operation succeeded (or already surfaced its own error); throw to
   * keep the dialog open — a reauthRequired throw shows the inline
   * "token rejected" message, any other throw closes the dialog (the caller's
   * own error handling owns the message).
   */
  replay: (confirmToken: string) => Promise<unknown> | unknown
}

type ReauthContextValue = {
  requestReauth: (request: ReauthRequest) => void
}

// Isolated component tests render sections without the app shell; a no-op
// default keeps them from crashing, while production mounts ReauthProvider in
// main.tsx so every rejected replay surfaces the dialog.
const ReauthContext = createContext<ReauthContextValue>({
  requestReauth: () => {},
})

export function ReauthProvider({ children }: { children: ReactNode }) {
  const { t } = useTranslation()
  const [pending, setPending] = useState<ReauthRequest | null>(null)
  const [errorMessage, setErrorMessage] = useState<string | null>(null)
  const [isPending, setIsPending] = useState(false)

  const requestReauth = useCallback((request: ReauthRequest) => {
    setPending(request)
    setErrorMessage(null)
  }, [])

  const handleOpenChange = useCallback(
    (open: boolean) => {
      // A replay is already in flight; keep the dialog until it settles.
      if (!open && !isPending) {
        setPending(null)
        setErrorMessage(null)
      }
    },
    [isPending]
  )

  const handleSubmit = useCallback(
    async (confirmToken: string) => {
      if (!pending || isPending) return
      setIsPending(true)
      setErrorMessage(null)
      try {
        await pending.replay(confirmToken)
        setPending(null)
      } catch (error) {
        if (isReauthRequired(error)) {
          setErrorMessage(t('reauth.errorInvalid'))
        } else {
          setPending(null)
        }
      } finally {
        setIsPending(false)
      }
    },
    [pending, isPending, t]
  )

  const value = useMemo(() => ({ requestReauth }), [requestReauth])

  return (
    <ReauthContext.Provider value={value}>
      {children}
      <ReauthDialog
        open={pending !== null}
        onOpenChange={handleOpenChange}
        description={pending?.description}
        errorMessage={errorMessage}
        isPending={isPending}
        onSubmit={(confirmToken) => {
          void handleSubmit(confirmToken)
        }}
      />
    </ReauthContext.Provider>
  )
}

export function useReauth(): ReauthContextValue {
  return useContext(ReauthContext)
}

/**
 * Shared onError handler for mutations whose endpoint may demand the master
 * token. Returns true when the error was a reauth demand (the dialog is now
 * handling it), so callers skip their ordinary failure toast.
 *
 * The `replaying` guard prevents a rejected replay from opening a second
 * prompt loop: the dialog already owns the inline error.
 */
export function useReauthReplay() {
  const { requestReauth } = useReauth()
  const replayingRef = useRef(false)

  return useCallback(
    (
      error: unknown,
      description: string,
      replay: (confirmToken: string) => Promise<unknown>
    ): boolean => {
      if (!isReauthRequired(error)) return false
      if (replayingRef.current) return true
      requestReauth({
        description,
        replay: async (confirmToken) => {
          replayingRef.current = true
          try {
            await replay(confirmToken)
          } finally {
            replayingRef.current = false
          }
        },
      })
      return true
    },
    [requestReauth]
  )
}
