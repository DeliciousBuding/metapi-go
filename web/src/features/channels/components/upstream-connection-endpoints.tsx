import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import type { ImportedUpstreamDetail } from '@/lib/api/imported-upstreams'

import type { ConnectionValues } from '../lib/upstream-config'
import { UpstreamEndpointsEditor } from './upstream-endpoints-editor'
import { UpstreamPresetEndpoints } from './upstream-preset-endpoints'

export function UpstreamConnectionEndpoints({
  detail,
  resolvingPreset,
  onResolvingChange,
  onManageModels,
}: {
  detail: ImportedUpstreamDetail
  resolvingPreset: boolean
  onResolvingChange: (resolving: boolean) => void
  onManageModels?: () => void
}) {
  const { t } = useTranslation()
  const form = useFormContext<ConnectionValues>()
  const { isSubmitting: pending, isDirty: dirty } = form.formState
  const legacyPaths =
    !Object.keys(detail.endpointConfig ?? {}).length &&
    [
      detail.openaiChatCompletionPath,
      detail.openaiResponsePath,
      detail.anthropicMessagePath,
    ].some(Boolean)
  return (
    <>
      <div className='space-y-3'>
        <FormField
          control={form.control}
          name='name'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('channels.upstream.name')}</FormLabel>
              <FormControl>
                <Input {...field} disabled={pending} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>
      <FormField
        control={form.control}
        name='baseUrl'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('channels.upstream.baseUrl')}</FormLabel>
            <FormControl>
              <Input {...field} disabled={pending} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <section className='space-y-4'>
        {legacyPaths ? (
          <p className='text-muted-foreground text-sm'>
            {t('channels.upstream.legacyPresetNote')}
          </p>
        ) : (
          <UpstreamPresetEndpoints
            baseUrl={form.watch('baseUrl')}
            disabled={pending}
            onResolvingChange={onResolvingChange}
            onApply={(endpoints) => {
              const existing = form.getValues('endpointConfig')
              const missing = Object.fromEntries(
                Object.entries(endpoints).filter(
                  ([key]) => !existing[key as keyof typeof existing]
                )
              )
              const count = Object.keys(missing).length
              if (count) {
                form.setValue(
                  'endpointConfig',
                  { ...existing, ...missing },
                  {
                    shouldDirty: true,
                    shouldValidate: true,
                  }
                )
              }
              return count
            }}
          />
        )}
        <FormField
          control={form.control}
          name='endpointConfig'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('channels.imported.endpoints')}</FormLabel>
              <FormDescription>
                {t('channels.upstream.endpointCapabilitiesHint')}
              </FormDescription>
              <FormControl>
                <div
                  className='min-w-0'
                  role='group'
                  aria-label={t('channels.imported.endpoints')}
                  ref={field.ref}
                >
                  <UpstreamEndpointsEditor
                    value={field.value}
                    onChange={field.onChange}
                    provider={detail.provider}
                    disabled={pending}
                  />
                </div>
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <div className='text-muted-foreground space-y-2 text-sm'>
          <p>{t('channels.upstream.modelAccessHint')}</p>
          {onManageModels && (
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={pending || resolvingPreset}
              onClick={onManageModels}
            >
              {t(
                dirty
                  ? 'channels.upstream.saveAndManageModels'
                  : 'channels.upstream.manageModels'
              )}
            </Button>
          )}
        </div>
        {!Object.keys(detail.endpointConfig ?? {}).length && (
          <div className='grid gap-3 sm:grid-cols-2'>
            {(
              [
                'openaiChatCompletionPath',
                'openaiResponsePath',
                'anthropicMessagePath',
              ] as const
            ).map((name) => (
              <FormField
                key={name}
                control={form.control}
                name={name}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t(`channels.upstream.${name}`)}</FormLabel>
                    <FormControl>
                      <Input {...field} disabled={pending} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            ))}
          </div>
        )}
      </section>
    </>
  )
}
