import type { Control } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

import type { ProxyTransportFormValues } from './proxy-transport-schema'

type FieldProps = { control: Control<ProxyTransportFormValues> }

export function ProxyRequestFields({ control }: FieldProps) {
  const { t } = useTranslation()
  return (
    <>
      <FormField
        control={control}
        name='systemProxyUrl'
        render={({ field }) => (
          <FormItem>
            <FormLabel>
              {t('settings.proxyModels.proxyTransport.fields.systemProxyUrl')}
            </FormLabel>
            <FormControl>
              <Input
                {...field}
                value={field.value ?? ''}
                placeholder='http://127.0.0.1:7890'
                className='font-mono'
              />
            </FormControl>
            <FormDescription>
              {t(
                'settings.proxyModels.proxyTransport.fields.systemProxyUrlHint'
              )}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={control}
        name='proxyErrorKeywords'
        render={({ field }) => (
          <FormItem>
            <FormLabel>
              {t(
                'settings.proxyModels.proxyTransport.fields.proxyErrorKeywords'
              )}
            </FormLabel>
            <FormControl>
              <Textarea
                {...field}
                value={field.value ?? ''}
                rows={4}
                className='font-mono'
              />
            </FormControl>
            <FormDescription>
              {t(
                'settings.proxyModels.proxyTransport.fields.proxyErrorKeywordsHint'
              )}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={control}
        name='proxyEmptyContentFailEnabled'
        render={({ field }) => (
          <FormItem className='flex flex-row items-center gap-3'>
            <FormControl>
              <Checkbox
                checked={Boolean(field.value)}
                onCheckedChange={field.onChange}
              />
            </FormControl>
            <div className='space-y-1'>
              <FormLabel className='cursor-pointer'>
                {t(
                  'settings.proxyModels.proxyTransport.fields.proxyEmptyContentFailEnabled'
                )}
              </FormLabel>
              <FormDescription>
                {t(
                  'settings.proxyModels.proxyTransport.fields.proxyEmptyContentFailEnabledHint'
                )}
              </FormDescription>
            </div>
          </FormItem>
        )}
      />
      <FormField
        control={control}
        name='payloadRules'
        render={({ field }) => (
          <FormItem>
            <FormLabel>
              {t('settings.proxyModels.proxyTransport.fields.payloadRules')}
            </FormLabel>
            <FormControl>
              <Textarea
                {...field}
                value={field.value ?? ''}
                rows={6}
                placeholder='{ "gpt-5.5": { "temperature": 0.7 } }'
                className='font-mono'
              />
            </FormControl>
            <FormDescription>
              {t('settings.proxyModels.proxyTransport.fields.payloadRulesHint')}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  )
}

export function ProxyAvailabilityFields({ control }: FieldProps) {
  const { t } = useTranslation()
  return (
    <>
      <div className='space-y-4 rounded-lg border p-4'>
        <div className='space-y-1'>
          <h3 className='text-sm font-medium'>
            {t('settings.proxyModels.proxyTransport.codexUpstreamGroup')}
          </h3>
          <p className='text-muted-foreground text-xs'>
            {t('settings.proxyModels.proxyTransport.codexUpstreamGroupHint')}
          </p>
        </div>
        <FormField
          control={control}
          name='codexUpstreamWebsocketEnabled'
          render={({ field }) => (
            <FormItem className='flex flex-row items-center gap-3'>
              <FormControl>
                <Checkbox
                  checked={Boolean(field.value)}
                  onCheckedChange={field.onChange}
                />
              </FormControl>
              <FormLabel className='cursor-pointer'>
                {t(
                  'settings.proxyModels.proxyTransport.fields.codexUpstreamWebsocketEnabled'
                )}
              </FormLabel>
            </FormItem>
          )}
        />
      </div>

      <FormField
        control={control}
        name='modelAvailabilityProbeEnabled'
        render={({ field }) => (
          <FormItem className='bg-muted/30 flex flex-row items-center gap-3 rounded-lg border p-4'>
            <FormControl>
              <Checkbox
                checked={Boolean(field.value)}
                onCheckedChange={field.onChange}
              />
            </FormControl>
            <div className='space-y-1'>
              <FormLabel className='cursor-pointer'>
                {t(
                  'settings.proxyModels.proxyTransport.fields.modelAvailabilityProbeEnabled'
                )}
              </FormLabel>
              <FormDescription>
                {t(
                  'settings.proxyModels.proxyTransport.fields.modelAvailabilityProbeEnabledHint'
                )}
              </FormDescription>
            </div>
          </FormItem>
        )}
      />
    </>
  )
}
