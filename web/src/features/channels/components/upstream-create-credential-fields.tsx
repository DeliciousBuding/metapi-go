import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import type { UpstreamPreset } from '@/lib/api/upstream-presets'

import type { UpstreamCreateValues } from '../lib/upstream-create'

export function UpstreamCreateCredentialFields({
  selected,
  pending,
  requiresBaseUrl,
}: {
  selected: UpstreamPreset
  pending: boolean
  requiresBaseUrl: boolean
}) {
  const { t } = useTranslation()
  const form = useFormContext<UpstreamCreateValues>()
  const customEndpoints = form.watch('customEndpoints')
  const credentialMode = selected.credentialMode ?? 'apiKey'
  return (
    <>
      {requiresBaseUrl && (
        <FormField
          control={form.control}
          name='baseUrl'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('channels.upstream.baseUrl')}</FormLabel>
              <FormControl>
                <Input {...field} placeholder='https://…' disabled={pending} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      )}
      {selected.requiresProject && (
        <FormField
          control={form.control}
          name='projectId'
          render={({ field }) => (
            <FormItem>
              <FormLabel>
                {t(
                  customEndpoints
                    ? 'channels.create.projectForPreset'
                    : 'channels.create.projectId'
                )}
              </FormLabel>
              <FormControl>
                <Input
                  {...field}
                  placeholder='my-gcp-project'
                  autoComplete='off'
                  disabled={pending}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      )}
      {credentialMode === 'gcpServiceAccount' ? (
        <FormField
          control={form.control}
          name='gcpServiceAccount'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('channels.upstream.gcpServiceAccount')}</FormLabel>
              <FormControl>
                <Textarea
                  {...field}
                  rows={8}
                  spellCheck={false}
                  autoComplete='off'
                  placeholder='{"type":"service_account",...}'
                  disabled={pending}
                  className='min-h-32 font-mono text-xs'
                />
              </FormControl>
              <p className='text-muted-foreground text-xs'>
                {t('channels.upstream.gcpServiceAccountHint')}
              </p>
              <FormMessage />
            </FormItem>
          )}
        />
      ) : (
        <FormField
          control={form.control}
          name='apiKey'
          render={({ field }) => (
            <FormItem>
              <FormLabel>
                {t(
                  credentialMode === 'optional'
                    ? 'channels.create.optionalKey'
                    : 'channels.create.apiKey'
                )}
              </FormLabel>
              <FormControl>
                <Input
                  {...field}
                  type='password'
                  autoComplete='new-password'
                  disabled={pending}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      )}
    </>
  )
}
