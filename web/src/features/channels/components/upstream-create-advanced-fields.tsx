import { ChevronDown } from 'lucide-react'
import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { StringMapEditor } from '@/components/common/string-map-editor'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import type { UpstreamPreset } from '@/lib/api/upstream-presets'

import type { UpstreamCreateValues } from '../lib/upstream-create'
import { ChannelProviderBinding } from './channel-provider-binding'

export function UpstreamCreateAdvancedFields({
  selected,
  pending,
  requiresBaseUrl,
  advanced,
  setAdvanced,
  resolvingEndpoints,
}: {
  selected: UpstreamPreset
  pending: boolean
  requiresBaseUrl: boolean
  advanced: boolean
  setAdvanced: (open: boolean) => void
  resolvingEndpoints: boolean
}) {
  const { t } = useTranslation()
  const form = useFormContext<UpstreamCreateValues>()
  const customEndpoints = form.watch('customEndpoints')
  const credentialMode = selected.credentialMode ?? 'apiKey'
  let providerCredentialKind: 'api_key' | 'none' | 'gcp_service_account' =
    'api_key'
  if (credentialMode === 'gcpServiceAccount')
    providerCredentialKind = 'gcp_service_account'
  else if (credentialMode === 'optional' && !form.watch('apiKey').trim())
    providerCredentialKind = 'none'
  return (
    <Collapsible
      open={advanced}
      onOpenChange={setAdvanced}
      className='rounded-xl border'
    >
      <CollapsibleTrigger className='group focus-visible:outline-ring flex w-full items-center justify-between gap-3 rounded-xl px-4 py-3 text-sm font-medium focus-visible:outline-2'>
        {t('channels.upstream.advanced')}
        <ChevronDown
          aria-hidden
          className='size-4 transition-transform group-data-[panel-open]:rotate-180'
        />
      </CollapsibleTrigger>
      <CollapsibleContent keepMounted className='space-y-4 border-t p-4'>
        <FormField
          control={form.control}
          name='name'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('channels.create.optionalName')}</FormLabel>
              <FormControl>
                <Input
                  {...field}
                  placeholder={selected.name}
                  disabled={pending}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        {!requiresBaseUrl && (
          <FormField
            control={form.control}
            name='baseUrl'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('channels.upstream.baseUrl')}</FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    placeholder={selected.defaultUrl}
                    disabled={pending}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        )}
        <FormField
          control={form.control}
          name='useSystemProxy'
          render={({ field }) => (
            <FormItem className='flex items-center justify-between gap-3'>
              <FormLabel>{t('channels.upstream.systemProxy')}</FormLabel>
              <FormControl>
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                  disabled={pending}
                />
              </FormControl>
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='channelProxy'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('channels.upstream.channelProxy')}</FormLabel>
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
        <FormField
          control={form.control}
          name='customHeaders'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('channels.upstream.customHeaders')}</FormLabel>
              <FormControl>
                <StringMapEditor
                  {...field}
                  ordered
                  structuredOnly
                  disabled={pending}
                  keyLabel={t('stringMapEditor.headerName')}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='providerBinding'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('channels.plugins.providerBinding')}</FormLabel>
              <FormDescription>
                {t('channels.plugins.providerDescription')}
              </FormDescription>
              <FormControl>
                <ChannelProviderBinding
                  {...field}
                  value={field.value ?? null}
                  protocols={
                    customEndpoints
                      ? Object.entries(form.watch('endpointConfig')).flatMap(
                          ([key, endpoint]) =>
                            endpoint
                              ? [key as UpstreamPreset['protocols'][number]]
                              : []
                        )
                      : form.watch('protocols')
                  }
                  credentialKind={providerCredentialKind}
                  newChannel
                  disabled={pending || resolvingEndpoints}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='paramOverride'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('channels.upstream.paramOverride')}</FormLabel>
              <FormControl>
                <StringMapEditor {...field} disabled={pending} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </CollapsibleContent>
    </Collapsible>
  )
}
