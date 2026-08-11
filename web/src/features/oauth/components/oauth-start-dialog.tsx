// metapi-go/features/oauth — start-authorization dialog (RHF + Zod + shadcn).
//
// A single dialog that starts an OAuth authorization flow. The user selects
// an enabled provider, optionally enters a project ID (required when the
// provider's `requiresProjectId` is true — enforced in the submit handler
// since the schema cannot see the runtime provider list), and optionally
// configures a proxy. On submit, `useStartOAuth` calls the backend and the
// returned `authorizationUrl` is opened in a new tab; the dialog closes and
// a toast instructs the user to complete the flow in the popup.

import { zodResolver } from '@hookform/resolvers/zod'
import { ExternalLink as ExternalLinkIcon } from 'lucide-react'
import { useEffect, useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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

import { useOAuthProviders, useStartOAuth } from '../api'
import {
  OAUTH_START_DEFAULT_VALUES,
  oauthStartSchema,
  type OAuthStartValues,
} from '../lib/oauth-schema'

type OAuthStartDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function OAuthStartDialog({
  open,
  onOpenChange,
}: OAuthStartDialogProps) {
  const { t } = useTranslation()
  const providersQuery = useOAuthProviders({ enabled: open })
  const startOAuth = useStartOAuth()

  const enabledProviders = useMemo(
    () => (providersQuery.data ?? []).filter((provider) => provider.enabled),
    [providersQuery.data]
  )

  const form = useForm<OAuthStartValues>({
    resolver: zodResolver(oauthStartSchema),
    defaultValues: OAUTH_START_DEFAULT_VALUES,
  })

  useEffect(() => {
    if (!open) return
    form.reset(OAUTH_START_DEFAULT_VALUES)
  }, [open, form])

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
      toast.success(t('oauth.form.startSucceeded'))
      onOpenChange(false)
    } catch {
      toast.error(t('oauth.form.startFailed'))
    }
  }

  const isSubmitting = startOAuth.isPending

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>{t('oauth.form.startTitle')}</DialogTitle>
          <DialogDescription>
            {t('oauth.form.startDescription')}
          </DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className='grid gap-4'>
            <FormField
              control={form.control}
              name='provider'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('oauth.form.provider')}</FormLabel>
                  <Select value={field.value} onValueChange={field.onChange}>
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue
                          placeholder={t('oauth.form.providerPlaceholder')}
                        />
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
                    <FormLabel>{t('oauth.form.useSystemProxy')}</FormLabel>
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
              <Button type='submit' disabled={isSubmitting}>
                {isSubmitting ? (
                  <Spinner className='mr-1' />
                ) : (
                  <ExternalLinkIcon className='mr-1 size-3.5' />
                )}
                {t('oauth.form.start')}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  )
}
