// metapi-go/features/settings/sections/general/components — proxy-transport
// section. Consolidates the legacy cards 3-7 (system proxy, error keywords,
// payload rules, codex upstream concurrency, model-availability probe) into
// one form. The legacy payload-rules visual editor is reduced to a JSON
// textarea here (functional, parse-validated); a richer editor can be
// layered back on later.

import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation } from '@tanstack/react-query'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import { api } from '@/lib/api'

import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'
import {
  asBoolean,
  asNumber,
  asString,
  joinListField,
  splitListField,
  useRuntimeSettings,
  useUpdateRuntimeSettings,
} from '../../../lib/runtime-settings'

const FORM_ID = 'settings-general-proxy-transport-form'

// payloadRules is a Record<string, unknown> on the wire; the textarea holds a
// JSON string. Zod refines it to a parseable object so we can show a friendly
// error before PUT.
const payloadRulesSchema = z
  .string()
  .optional()
  .refine(
    (value) => {
      if (!value) {
        return true
      }
      try {
        const parsed = JSON.parse(value)
        return typeof parsed === 'object' && parsed !== null
      } catch {
        return false
      }
    },
    { message: 'settings.general.proxyTransport.schema.payloadRulesInvalid' }
  )

const proxyTransportSchema = z.object({
  systemProxyUrl: z.string().optional(),
  proxyErrorKeywords: z.string().optional(),
  proxyEmptyContentFailEnabled: z.boolean().optional(),
  payloadRules: payloadRulesSchema,
  codexUpstreamWebsocketEnabled: z.boolean().optional(),
  responsesCompactFallbackToResponsesEnabled: z.boolean().optional(),
  proxySessionChannelConcurrencyLimit: z.coerce
    .number()
    .int()
    .min(0)
    .optional(),
  proxySessionChannelQueueWaitMs: z.coerce.number().int().min(0).optional(),
  modelAvailabilityProbeEnabled: z.boolean().optional(),
})

type ProxyTransportFormValues = z.infer<typeof proxyTransportSchema>

export function ProxyTransportSection() {
  const { t } = useTranslation()
  const { data, isLoading } = useRuntimeSettings()
  const updateMutation = useUpdateRuntimeSettings()

  const form = useForm<ProxyTransportFormValues>({
    resolver: zodResolver(proxyTransportSchema) as never,
    defaultValues: {
      systemProxyUrl: '',
      proxyErrorKeywords: '',
      proxyEmptyContentFailEnabled: false,
      payloadRules: '',
      codexUpstreamWebsocketEnabled: false,
      responsesCompactFallbackToResponsesEnabled: false,
      proxySessionChannelConcurrencyLimit: 2,
      proxySessionChannelQueueWaitMs: 1500,
      modelAvailabilityProbeEnabled: false,
    },
  })

  useEffect(() => {
    if (!data) {
      return
    }
    const payloadRulesRaw = data.payloadRules
    let payloadRulesJson = ''
    if (payloadRulesRaw && typeof payloadRulesRaw === 'object') {
      try {
        payloadRulesJson = JSON.stringify(payloadRulesRaw, null, 2)
      } catch {
        payloadRulesJson = ''
      }
    }
    form.reset(
      {
        systemProxyUrl: asString(data.systemProxyUrl),
        proxyErrorKeywords: joinListField(
          splitListField(data.proxyErrorKeywords)
        ),
        proxyEmptyContentFailEnabled: asBoolean(
          data.proxyEmptyContentFailEnabled
        ),
        payloadRules: payloadRulesJson,
        codexUpstreamWebsocketEnabled: asBoolean(
          data.codexUpstreamWebsocketEnabled
        ),
        responsesCompactFallbackToResponsesEnabled: asBoolean(
          data.responsesCompactFallbackToResponsesEnabled
        ),
        proxySessionChannelConcurrencyLimit:
          asNumber(data.proxySessionChannelConcurrencyLimit) ?? 2,
        proxySessionChannelQueueWaitMs:
          asNumber(data.proxySessionChannelQueueWaitMs) ?? 1500,
        modelAvailabilityProbeEnabled: asBoolean(
          data.modelAvailabilityProbeEnabled
        ),
      },
      { keepDirtyValues: true }
    )
  }, [data, form])

  const testProxyMutation = useMutation({
    mutationFn: async (proxyUrl: string) => api.testSystemProxy({ proxyUrl }),
    onSuccess: (result) => {
      const latency = (result as { latencyMs?: number } | null)?.latencyMs
      if (typeof latency === 'number') {
        toast.success(
          t('settings.general.proxyTransport.toast.proxyOk', { ms: latency })
        )
      } else {
        toast.success(t('settings.general.proxyTransport.toast.proxyOkGeneric'))
      }
    },
    onError: () =>
      toast.error(t('settings.general.proxyTransport.toast.proxyFailed')),
  })

  function onSubmit(values: ProxyTransportFormValues) {
    const { payloadRules, proxyErrorKeywords, ...rest } = values
    const payload: Record<string, unknown> = { ...rest }
    if (proxyErrorKeywords !== undefined) {
      payload.proxyErrorKeywords = splitListField(proxyErrorKeywords)
    }
    if (payloadRules !== undefined) {
      // Empty string → send null to clear; valid JSON → parsed object.
      payload.payloadRules = payloadRules
        ? (JSON.parse(payloadRules) as Record<string, unknown>)
        : null
    }
    updateMutation.mutate(payload as never, {
      onSuccess: () =>
        toast.success(t('settings.general.proxyTransport.toast.saved')),
      onError: () =>
        toast.error(t('settings.general.proxyTransport.toast.saveFailed')),
    })
  }

  if (isLoading) {
    return <SettingsSectionSkeleton />
  }

  const systemProxyUrl = form.watch('systemProxyUrl')

  return (
    <SettingsSectionCard
      title={t('settings.general.proxyTransport.title')}
      description={t('settings.general.proxyTransport.description')}
    >
      <Form {...form}>
        <form
          id={FORM_ID}
          onSubmit={form.handleSubmit(onSubmit)}
          className='space-y-6'
        >
          <FormField
            control={form.control}
            name='systemProxyUrl'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('settings.general.proxyTransport.fields.systemProxyUrl')}
                </FormLabel>
                <div className='flex gap-2'>
                  <FormControl>
                    <Input
                      {...field}
                      value={field.value ?? ''}
                      placeholder='http://127.0.0.1:7890'
                      className='font-mono'
                    />
                  </FormControl>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    disabled={testProxyMutation.isPending || !systemProxyUrl}
                    onClick={() =>
                      testProxyMutation.mutate(systemProxyUrl ?? '')
                    }
                  >
                    {testProxyMutation.isPending
                      ? t('settings.common.testing')
                      : t('settings.general.proxyTransport.testProxy')}
                  </Button>
                </div>
                <FormDescription>
                  {t(
                    'settings.general.proxyTransport.fields.systemProxyUrlHint'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className='space-y-4 rounded-lg border p-4'>
            <h4 className='text-sm font-medium'>
              {t('settings.general.proxyTransport.errorRulesGroup')}
            </h4>
            <FormField
              control={form.control}
              name='proxyErrorKeywords'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t(
                      'settings.general.proxyTransport.fields.proxyErrorKeywords'
                    )}
                  </FormLabel>
                  <FormControl>
                    <textarea
                      {...field}
                      value={field.value ?? ''}
                      rows={4}
                      placeholder='rate limit&#10;blocked'
                      className='border-input placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-ring/50 flex field-sizing-content w-full rounded-md border bg-transparent px-3 py-2 text-sm shadow-xs transition-[color,box-shadow] outline-none focus-visible:ring-[3px]'
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'settings.general.proxyTransport.fields.proxyErrorKeywordsHint'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
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
                        'settings.general.proxyTransport.fields.proxyEmptyContentFailEnabled'
                      )}
                    </FormLabel>
                    <FormDescription>
                      {t(
                        'settings.general.proxyTransport.fields.proxyEmptyContentFailEnabledHint'
                      )}
                    </FormDescription>
                  </div>
                </FormItem>
              )}
            />
          </div>

          <FormField
            control={form.control}
            name='payloadRules'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('settings.general.proxyTransport.fields.payloadRules')}
                </FormLabel>
                <FormControl>
                  <textarea
                    {...field}
                    value={field.value ?? ''}
                    rows={8}
                    placeholder='{ "default": {} }'
                    className='border-input placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-ring/50 flex field-sizing-content w-full rounded-md border bg-transparent px-3 py-2 font-mono text-sm shadow-xs transition-[color,box-shadow] outline-none focus-visible:ring-[3px]'
                  />
                </FormControl>
                <FormDescription>
                  {t('settings.general.proxyTransport.fields.payloadRulesHint')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className='space-y-4 rounded-lg border p-4'>
            <h4 className='text-sm font-medium'>
              {t('settings.general.proxyTransport.codexUpstreamGroup')}
            </h4>
            <FormField
              control={form.control}
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
                      'settings.general.proxyTransport.fields.codexUpstreamWebsocketEnabled'
                    )}
                  </FormLabel>
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='responsesCompactFallbackToResponsesEnabled'
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
                      'settings.general.proxyTransport.fields.responsesCompactFallbackEnabled'
                    )}
                  </FormLabel>
                </FormItem>
              )}
            />
            <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='proxySessionChannelConcurrencyLimit'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t(
                        'settings.general.proxyTransport.fields.proxySessionChannelConcurrencyLimit'
                      )}
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? ''}
                        type='number'
                        min={0}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='proxySessionChannelQueueWaitMs'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t(
                        'settings.general.proxyTransport.fields.proxySessionChannelQueueWaitMs'
                      )}
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? ''}
                        type='number'
                        min={0}
                        step={100}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          </div>

          <FormField
            control={form.control}
            name='modelAvailabilityProbeEnabled'
            render={({ field }) => (
              <FormItem className='border-destructive/40 bg-destructive/5 flex flex-row items-center gap-3 rounded-lg border p-4'>
                <FormControl>
                  <Checkbox
                    checked={Boolean(field.value)}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
                <div className='space-y-1'>
                  <FormLabel className='cursor-pointer'>
                    {t(
                      'settings.general.proxyTransport.fields.modelAvailabilityProbeEnabled'
                    )}
                  </FormLabel>
                  <FormDescription>
                    {t(
                      'settings.general.proxyTransport.fields.modelAvailabilityProbeEnabledHint'
                    )}
                  </FormDescription>
                </div>
              </FormItem>
            )}
          />

          <Button
            type='submit'
            form={FORM_ID}
            disabled={updateMutation.isPending}
          >
            {updateMutation.isPending
              ? t('settings.common.saving')
              : t('settings.common.save')}
          </Button>
        </form>
      </Form>
    </SettingsSectionCard>
  )
}
