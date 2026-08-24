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

import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  Check as CheckIcon,
  Copy as CopyIcon,
  ExternalLink as ExternalLinkIcon,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/lib/toast'

import {
  useOAuthProviders,
  useStartOAuth,
  useSubmitOAuthManualCallback,
} from '../api'
import {
  OAUTH_START_DEFAULT_VALUES,
  oauthStartSchema,
  type OAuthStartValues,
} from '../lib/oauth-schema'
import {
  OAUTH_SESSION_POLL_MAX_ATTEMPTS,
  useOAuthSessionPolling,
} from '../lib/oauth-session-polling'
import { oauthKeys, type OAuthStartInstructions } from '../types'

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
  const providersQuery = useOAuthProviders({ enabled: open })
  const startOAuth = useStartOAuth()

  // Pending-session state — set after `useStartOAuth` returns a `state`. While
  // set, the dialog swaps the form for a pending panel that polls the
  // session and offers a manual-callback fallback.
  const [pendingState, setPendingState] = useState<string | null>(null)
  const [instructions, setInstructions] =
    useState<OAuthStartInstructions | null>(null)
  const [callbackUrl, setCallbackUrl] = useState('')
  // Which copyable field ('state' | 'redirectUri' | 'sshTunnel' |
  // 'sshTunnelKey') last copied — drives the transient check icon.
  const [copiedField, setCopiedField] = useState<string | null>(null)
  // Inline validation error for the manual-callback input (not a valid
  // http(s) URL). Cleared as soon as the user edits the field.
  const [callbackInvalid, setCallbackInvalid] = useState(false)
  // Closing while a session is pending would silently abandon the OAuth wait,
  // so every close affordance (X / Escape / overlay / Cancel) first routes
  // through the abandon confirmation.
  const [confirmAbandonOpen, setConfirmAbandonOpen] = useState(false)

  const { session, exhausted, kick } = useOAuthSessionPolling(pendingState)
  const submitManualCallback = useSubmitOAuthManualCallback()

  const enabledProviders = useMemo(
    () => (providersQuery.data ?? []).filter((provider) => provider.enabled),
    [providersQuery.data]
  )

  // The provider <Select> collapses three distinct non-ready states into an
  // empty dropdown otherwise. Branch on the query state so the user sees
  // why there is nothing to pick and how to recover, instead of clicking
  // Start and getting a bare "Please select a provider." validation error.
  const providersLoading = providersQuery.isLoading
  const providersError = providersQuery.isError
  const hasEnabledProviders = enabledProviders.length > 0
  const providersReady =
    !providersLoading && !providersError && hasEnabledProviders

  const form = useForm<OAuthStartValues>({
    resolver: zodResolver(oauthStartSchema),
    defaultValues: OAUTH_START_DEFAULT_VALUES,
  })

  useEffect(() => {
    if (!open) return
    form.reset(OAUTH_START_DEFAULT_VALUES)
  }, [open, form])

  // Drop pending state when the dialog closes (user cancelled mid-flow) so
  // polling stops and the form re-renders fresh on next open.
  useEffect(() => {
    if (!open && pendingState) {
      setPendingState(null)
      setInstructions(null)
      setCallbackUrl('')
      setCallbackInvalid(false)
    }
  }, [open, pendingState])

  // React to the polled session status. On success, toast + invalidate the
  // connections list (a new connection is now created) + close the dialog.
  // On error, the panel surfaces the error string but stays open so the user
  // can paste a corrected callback URL.
  const sessionStatus = session?.status
  useEffect(() => {
    if (!pendingState) return
    if (sessionStatus === 'success') {
      toast.success(t('oauth.session.succeeded'))
      void queryClient.invalidateQueries({
        queryKey: oauthKeys.connections(),
      })
      setPendingState(null)
      setInstructions(null)
      setCallbackUrl('')
      setCallbackInvalid(false)
      onOpenChange(false)
    }
  }, [pendingState, sessionStatus, t, onOpenChange, queryClient])

  const selectedProviderId = form.watch('provider')
  const selectedProvider = useMemo(
    () =>
      enabledProviders.find(
        (provider) => provider.provider === selectedProviderId
      ),
    [enabledProviders, selectedProviderId]
  )
  const requiresProjectId = selectedProvider?.requiresProjectId ?? false

  async function onSubmit(values: OAuthStartValues) {
    if (requiresProjectId && !values.projectId.trim()) {
      form.setError('projectId', {
        type: 'manual',
        message: t('oauth.form.errors.projectIdRequired'),
      })
      return
    }
    try {
      const result = await startOAuth.mutateAsync({
        provider: values.provider,
        projectId: values.projectId.trim() || undefined,
        proxyUrl: values.proxyUrl.trim() || null,
        useSystemProxy: values.useSystemProxy,
      })
      if (result.authorizationUrl) {
        window.open(result.authorizationUrl, '_blank', 'noopener,noreferrer')
      }
      // Keep the dialog open and switch to the pending panel: the backend
      // creates the connection only after the OAuth callback completes.
      setInstructions(result.instructions)
      setCallbackInvalid(false)
      setPendingState(result.state)
    } catch {
      toast.error(t('oauth.form.startFailed'))
    }
  }

  async function handleCopy(field: string, text: string) {
    try {
      await navigator.clipboard.writeText(text)
      setCopiedField(field)
      setTimeout(
        () => setCopiedField((current) => (current === field ? null : current)),
        1500
      )
    } catch {
      // Clipboard may be unavailable (non-secure context / permissions).
      toast.error(t('common.copyFailed'))
    }
  }

  function isValidCallbackUrl(value: string): boolean {
    try {
      const parsed = new URL(value)
      return parsed.protocol === 'http:' || parsed.protocol === 'https:'
    } catch {
      return false
    }
  }

  async function handleManualCallbackSubmit(
    event: React.FormEvent<HTMLFormElement>
  ) {
    event.preventDefault()
    if (!pendingState) return
    const url = callbackUrl.trim()
    if (!url) return
    if (!isValidCallbackUrl(url)) {
      setCallbackInvalid(true)
      return
    }
    setCallbackInvalid(false)
    try {
      await submitManualCallback.mutateAsync({
        state: pendingState,
        callbackUrl: url,
      })
      setCallbackUrl('')
      toast.success(t('oauth.session.callbackSubmitted'))
      // The backend settles (or advances) the session on manual callback —
      // re-check immediately with a fresh budget instead of waiting out the
      // interval.
      kick()
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      toast.error(message || t('common.requestFailed'))
    }
  }

  const isSubmitting = startOAuth.isPending
  const isSubmittingCallback = submitManualCallback.isPending

  // Hoisted so the `string` narrowing survives inside the copy-button
  // closures (optional-property narrowing does not cross closures).
  const redirectUri = instructions?.redirectUri
  const sshTunnelCommand = instructions?.sshTunnelCommand
  const sshTunnelKeyCommand = instructions?.sshTunnelKeyCommand

  // Pending-panel status row: backend error > honest exhaustion > waiting.
  let statusPanel: React.ReactNode
  if (sessionStatus === 'error') {
    statusPanel = (
      <div className='text-destructive text-sm'>
        {t('oauth.session.failed', {
          error: session?.error ?? '',
        })}
      </div>
    )
  } else if (exhausted) {
    // Honest timeout: polling paused, the wait is NOT over.
    statusPanel = (
      <div className='text-warning-soft-fg text-sm' role='status'>
        {t('oauth.session.pollingExhausted', {
          attempts: OAUTH_SESSION_POLL_MAX_ATTEMPTS,
        })}
      </div>
    )
  } else {
    statusPanel = (
      <div className='text-muted-foreground flex items-center gap-2 text-sm'>
        <Spinner className='size-3.5' />
        <span>{t('oauth.session.waiting')}</span>
      </div>
    )
  }

  function requestClose() {
    if (pendingState) {
      setConfirmAbandonOpen(true)
      return
    }
    onOpenChange(false)
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
            <>
              <DialogHeader>
                <DialogTitle>{t('oauth.session.waiting')}</DialogTitle>
                <DialogDescription>
                  {t('oauth.session.waitingDescription')}
                </DialogDescription>
              </DialogHeader>

              <div className='grid gap-4'>
                {statusPanel}

                <div className='grid gap-2'>
                  <p className='text-muted-foreground text-sm'>
                    {t('oauth.session.stateHint')}
                  </p>
                  <div className='flex items-center gap-2'>
                    <code className='bg-muted flex-1 overflow-x-auto rounded px-2 py-1.5 text-xs'>
                      {pendingState}
                    </code>
                    <Button
                      type='button'
                      variant='outline'
                      size='icon-sm'
                      onClick={() => handleCopy('state', pendingState)}
                      aria-label={t('common.copy')}
                    >
                      {copiedField === 'state' ? (
                        <CheckIcon className='size-3.5' />
                      ) : (
                        <CopyIcon className='size-3.5' />
                      )}
                    </Button>
                  </div>
                </div>

                {redirectUri && (
                  <div className='grid gap-2'>
                    <p className='text-muted-foreground text-sm'>
                      {t('oauth.session.redirectUriHint')}
                    </p>
                    <div className='flex items-center gap-2'>
                      <code className='bg-muted flex-1 overflow-x-auto rounded px-2 py-1.5 text-xs'>
                        {redirectUri}
                      </code>
                      <Button
                        type='button'
                        variant='outline'
                        size='icon-sm'
                        onClick={() => handleCopy('redirectUri', redirectUri)}
                        aria-label={t('common.copy')}
                      >
                        {copiedField === 'redirectUri' ? (
                          <CheckIcon className='size-3.5' />
                        ) : (
                          <CopyIcon className='size-3.5' />
                        )}
                      </Button>
                    </div>
                  </div>
                )}

                {sshTunnelCommand && (
                  <div className='grid gap-2'>
                    <p className='text-muted-foreground text-sm'>
                      {t('oauth.session.sshTunnelHint')}
                    </p>
                    <div className='flex items-center gap-2'>
                      <code className='bg-muted flex-1 overflow-x-auto rounded px-2 py-1.5 text-xs'>
                        {sshTunnelCommand}
                      </code>
                      <Button
                        type='button'
                        variant='outline'
                        size='icon-sm'
                        onClick={() =>
                          handleCopy('sshTunnel', sshTunnelCommand)
                        }
                        aria-label={t('common.copy')}
                      >
                        {copiedField === 'sshTunnel' ? (
                          <CheckIcon className='size-3.5' />
                        ) : (
                          <CopyIcon className='size-3.5' />
                        )}
                      </Button>
                    </div>
                    {sshTunnelKeyCommand && (
                      <>
                        <p className='text-muted-foreground text-sm'>
                          {t('oauth.session.sshTunnelKeyHint')}
                        </p>
                        <div className='flex items-center gap-2'>
                          <code className='bg-muted flex-1 overflow-x-auto rounded px-2 py-1.5 text-xs'>
                            {sshTunnelKeyCommand}
                          </code>
                          <Button
                            type='button'
                            variant='outline'
                            size='icon-sm'
                            onClick={() =>
                              handleCopy('sshTunnelKey', sshTunnelKeyCommand)
                            }
                            aria-label={t('common.copy')}
                          >
                            {copiedField === 'sshTunnelKey' ? (
                              <CheckIcon className='size-3.5' />
                            ) : (
                              <CopyIcon className='size-3.5' />
                            )}
                          </Button>
                        </div>
                      </>
                    )}
                  </div>
                )}

                <form
                  onSubmit={handleManualCallbackSubmit}
                  className='grid gap-2'
                >
                  <label
                    className='text-sm font-medium'
                    htmlFor='oauth-manual-callback-url'
                  >
                    {t('oauth.session.callbackLabel')}
                  </label>
                  <Input
                    id='oauth-manual-callback-url'
                    value={callbackUrl}
                    onChange={(event) => {
                      setCallbackUrl(event.target.value)
                      setCallbackInvalid(false)
                    }}
                    placeholder={t('oauth.session.callbackPlaceholder')}
                    autoComplete='off'
                    spellCheck={false}
                    aria-invalid={callbackInvalid || undefined}
                  />
                  {callbackInvalid && (
                    <p className='text-destructive text-xs'>
                      {t('oauth.session.callbackInvalidUrl')}
                    </p>
                  )}
                  <Button
                    type='submit'
                    disabled={!callbackUrl.trim() || isSubmittingCallback}
                  >
                    {isSubmittingCallback ? <Spinner /> : null}
                    {t('oauth.session.callbackSubmit')}
                  </Button>
                </form>

                <DialogFooter>
                  <Button
                    type='button'
                    variant='outline'
                    onClick={requestClose}
                  >
                    {t('oauth.form.cancel')}
                  </Button>
                </DialogFooter>
              </div>
            </>
          ) : (
            <>
              <DialogHeader>
                <DialogTitle>{t('oauth.form.startTitle')}</DialogTitle>
                <DialogDescription>
                  {t('oauth.form.startDescription')}
                </DialogDescription>
              </DialogHeader>

              <Form {...form}>
                <form
                  onSubmit={form.handleSubmit(onSubmit)}
                  className='grid gap-4'
                >
                  <FormField
                    control={form.control}
                    name='provider'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('oauth.form.provider')}</FormLabel>
                        <Select
                          value={field.value}
                          onValueChange={field.onChange}
                        >
                          <FormControl>
                            <SelectTrigger disabled={!providersReady}>
                              <SelectValue>
                                {(selected) => {
                                  if (!selected) {
                                    return t('oauth.form.providerPlaceholder')
                                  }
                                  const provider = enabledProviders.find(
                                    (item) => item.provider === selected
                                  )
                                  return provider
                                    ? provider.label || provider.provider
                                    : String(selected)
                                }}
                              </SelectValue>
                            </SelectTrigger>
                          </FormControl>
                          <SelectContent>
                            {enabledProviders.map((provider) => (
                              <SelectItem
                                key={provider.provider}
                                value={provider.provider}
                              >
                                {provider.label || provider.provider}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                        {providersLoading && (
                          <div className='text-muted-foreground flex items-center gap-1.5 text-sm'>
                            <Spinner className='size-3.5' />
                            {t('oauth.form.loadingProviders')}
                          </div>
                        )}
                        {providersError && (
                          <div className='text-destructive flex items-center gap-2 text-sm'>
                            <span>{t('oauth.form.providersLoadFailed')}</span>
                            <Button
                              type='button'
                              variant='link'
                              size='sm'
                              className='h-auto px-0'
                              onClick={() => providersQuery.refetch()}
                            >
                              {t('oauth.form.retry')}
                            </Button>
                          </div>
                        )}
                        {!providersLoading &&
                          !providersError &&
                          !hasEnabledProviders && (
                            <FormDescription>
                              {t('oauth.form.noProviders')}{' '}
                              <Link
                                to='/settings'
                                className='text-primary hover:underline'
                              >
                                {t('oauth.form.goToSettings')}
                              </Link>
                            </FormDescription>
                          )}
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  {requiresProjectId && (
                    <FormField
                      control={form.control}
                      name='projectId'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('oauth.form.projectId')}</FormLabel>
                          <FormControl>
                            <Input
                              placeholder={t('oauth.form.projectIdPlaceholder')}
                              autoFocus
                              {...field}
                            />
                          </FormControl>
                          <FormDescription>
                            {t('oauth.form.projectIdDescription')}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  )}

                  <FormField
                    control={form.control}
                    name='proxyUrl'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('oauth.form.proxyUrl')}</FormLabel>
                        <FormControl>
                          <Input
                            placeholder={t('oauth.form.optionalUrlPlaceholder')}
                            {...field}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='useSystemProxy'
                    render={({ field }) => (
                      <FormItem className='border-border flex flex-row items-center justify-between rounded-lg border p-3'>
                        <div className='space-y-0.5'>
                          <FormLabel>
                            {t('oauth.form.useSystemProxy')}
                          </FormLabel>
                          <FormDescription>
                            {t('oauth.form.useSystemProxyDescription')}
                          </FormDescription>
                        </div>
                        <FormControl>
                          <Switch
                            checked={field.value}
                            onCheckedChange={field.onChange}
                          />
                        </FormControl>
                      </FormItem>
                    )}
                  />

                  <DialogFooter>
                    <Button
                      type='button'
                      variant='outline'
                      onClick={() => onOpenChange(false)}
                      disabled={isSubmitting}
                    >
                      {t('oauth.form.cancel')}
                    </Button>
                    <Button
                      type='submit'
                      disabled={isSubmitting || !providersReady}
                    >
                      {isSubmitting ? (
                        <Spinner />
                      ) : (
                        <ExternalLinkIcon className='size-4' />
                      )}
                      {t('oauth.form.start')}
                    </Button>
                  </DialogFooter>
                </form>
              </Form>
            </>
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
    </>
  )
}
