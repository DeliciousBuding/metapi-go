// metapi-go/features/oauth — start-authorization dialog (RHF + Zod + shadcn).
//
// A single dialog that starts an OAuth authorization flow. The user selects
// an enabled provider, optionally enters a project ID (required when the
// provider's `requiresProjectId` is true — enforced in the submit handler
// since the schema cannot see the runtime provider list), and optionally
// configures a proxy. On submit, `useStartOAuth` calls the backend and the
// returned `authorizationUrl` is opened in a new tab. The dialog does NOT
// close: it switches to a pending panel that shows the returned
// `instructions` + `state` (both copyable) and polls
// `useOAuthSessionPolling(state)` with a bounded attempt budget until the
// backend reports success/error. When the budget runs out the panel says so
// honestly ("still waiting — paste the callback manually") instead of
// pretending success, and a manual-callback fallback (paste the redirect
// URL) stays available for environments where the callback port is not
// reachable from the browser.

import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
import { Dialog, DialogContent } from '@/components/ui/dialog'
import { toast } from '@/lib/toast'

import { useOAuthSessionPolling } from '../lib/oauth-session-polling'
import { oauthKeys, type OAuthStartInstructions } from '../types'
import { useManualCallback } from './oauth-manual-callback'
import { OAuthSessionPanel, useInstructionCopy } from './oauth-session-panel'
import { OAuthStartForm, useOAuthStartForm } from './oauth-start-form'

type OAuthStartDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function OAuthStartDialog({
  open,
  onOpenChange,
}: OAuthStartDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  // Pending-session state — set after `useStartOAuth` returns a `state`. While
  // set, the dialog swaps the form for a pending panel that polls the
  // session and offers a manual-callback fallback.
  const [pendingState, setPendingState] = useState<string | null>(null)
  const [instructions, setInstructions] =
    useState<OAuthStartInstructions | null>(null)
  // The authorization URL returned by the backend. It is opened in a new tab
  // on submit, but that window.open fires AFTER the startOAuth network
  // round-trip — outside the browser's transient-activation window — so a
  // popup blocker can silently swallow it. The URL is therefore kept in
  // state and rendered as a link in the pending panel so the user can always
  // open it manually.
  const [authorizationUrl, setAuthorizationUrl] = useState<string | null>(null)
  // Closing while a session is pending would silently abandon the OAuth wait,
  // so every close affordance (X / Escape / overlay / Cancel) first routes
  // through the abandon confirmation.
  const [confirmAbandonOpen, setConfirmAbandonOpen] = useState(false)

  const deviceFlow = instructions?.deviceFlow ?? null
  const devicePollIntervalSeconds =
    deviceFlow?.pollIntervalSeconds && deviceFlow.pollIntervalSeconds > 0
      ? deviceFlow.pollIntervalSeconds
      : 5
  const pollIntervalMs = deviceFlow
    ? devicePollIntervalSeconds * 1000
    : undefined
  const pollMaxAttempts = deviceFlow
    ? Math.ceil(
        Math.max(deviceFlow.expiresInSeconds, devicePollIntervalSeconds) /
          devicePollIntervalSeconds
      ) + 1
    : undefined
  const polling = useOAuthSessionPolling(
    pendingState,
    pollIntervalMs,
    pollMaxAttempts
  )
  const copy = useInstructionCopy()
  const callback = useManualCallback(pendingState, polling.kick)
  const { reset: resetCallback } = callback
  const clearSession = useCallback(() => {
    setPendingState(null)
    setInstructions(null)
    setAuthorizationUrl(null)
    resetCallback()
  }, [resetCallback])
  const start = useOAuthStartForm(open, (result) => {
    if (result.authorizationUrl) {
      setAuthorizationUrl(result.authorizationUrl)
      window.open(result.authorizationUrl, '_blank', 'noopener,noreferrer')
    }
    // Keep the dialog open and switch to the pending panel: the backend
    // creates the connection only after the OAuth callback completes.
    setInstructions(result.instructions)
    callback.clearError()
    setPendingState(result.state)
  })
  const { form } = start
  // The form view closes silently on dirty edits otherwise; intercept the
  // close and offer the shared discard confirmation (the pending panel has
  // its own abandon flow, so the guard only applies to the form view).
  const { handleOpenChange, guard: dirtyGuard } = useDirtyDialogClose({
    enabled: !pendingState && form.formState.isDirty,
    onDiscard: () => form.reset(),
    onOpenChange,
  })

  // Drop pending state when the dialog closes (user cancelled mid-flow) so
  // polling stops and the form re-renders fresh on next open.
  useEffect(() => {
    if (!open && pendingState) clearSession()
  }, [open, pendingState, clearSession])

  // React to the polled session status. On success, toast + invalidate the
  // connections list (a new connection is now created) + close the dialog.
  // On error, the panel surfaces the error string but stays open so the user
  // can paste a corrected callback URL.
  const sessionStatus = polling.session?.status
  useEffect(() => {
    if (!pendingState) return
    if (sessionStatus === 'success') {
      toast.success(t('oauth.session.succeeded'))
      void queryClient.invalidateQueries({
        queryKey: oauthKeys.connections(),
      })
      clearSession()
      onOpenChange(false)
    }
  }, [pendingState, sessionStatus, t, onOpenChange, queryClient, clearSession])

  function requestClose() {
    if (pendingState) {
      setConfirmAbandonOpen(true)
      return
    }
    handleOpenChange(false)
  }

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={(next) => {
          if (next) {
            onOpenChange(true)
          } else {
            requestClose()
          }
        }}
      >
        <DialogContent className='sm:max-w-lg'>
          {pendingState ? (
            <OAuthSessionPanel
              pendingState={pendingState}
              instructions={instructions}
              authorizationUrl={authorizationUrl}
              polling={polling}
              copy={copy}
              callback={callback}
              requestClose={requestClose}
            />
          ) : (
            <OAuthStartForm start={start} requestClose={requestClose} />
          )}
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={confirmAbandonOpen}
        title={t('oauth.session.abandonTitle')}
        description={t('oauth.session.abandonDescription')}
        confirmLabel={t('oauth.session.abandonConfirm')}
        cancelLabel={t('oauth.session.keepWaiting')}
        destructive
        onConfirm={() => {
          setConfirmAbandonOpen(false)
          onOpenChange(false)
        }}
        onCancel={() => setConfirmAbandonOpen(false)}
      />

      {dirtyGuard}
    </>
  )
}
