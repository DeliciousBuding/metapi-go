// metapi-go/features/settings/sections/basic/components — proxy-transport
// section. Consolidates the legacy cards 3-7 (system proxy, error keywords,
// payload rules, codex upstream concurrency, model-availability probe) into
// one form. The legacy payload-rules visual editor is reduced to a JSON
// textarea here (functional, parse-validated).

import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
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
import { Textarea } from '@/components/ui/textarea'
import { api } from '@/lib/api'
import { toast } from '@/lib/toast'

import { FormNavigationGuard } from '../../../components/form-navigation-guard'
import { SettingsFormActions } from '../../../components/settings-form-actions'
import { SettingsSectionCard } from '../../../components/settings-section-card'
import { SettingsSectionError } from '../../../components/settings-section-error'
import { useSettingsForm } from '../../../hooks/use-settings-form'
import {
  collectChangedFields,
  hasChanges,
} from '../../../lib/collect-changed-fields'
import {
  asBoolean,
  asNumber,
  asString,
  joinListField,
  splitListField,
  useRuntimeSettings,
  useUpdateRuntimeSettings,
  type RuntimeSettings,
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
    {
      message: 'settings.proxyModels.proxyTransport.schema.payloadRulesInvalid',
    }
  )

const proxyTransportSchema = z.object({
  systemProxyUrl: z.string().optional(),
  proxyErrorKeywords: z.string().optional(),
  proxyEmptyContentFailEnabled: z.boolean(),
  payloadRules: payloadRulesSchema,
  codexUpstreamWebsocketEnabled: z.boolean(),
  responsesCompactFallbackToResponsesEnabled: z.boolean(),
  proxySessionChannelConcurrencyLimit: z.coerce.number().int().min(0),
  proxySessionChannelQueueWaitMs: z.coerce.number().int().min(0),
  modelAvailabilityProbeEnabled: z.boolean(),
})

type ProxyTransportFormValues = z.infer<typeof proxyTransportSchema>

const DEFAULT_VALUES: ProxyTransportFormValues = {
  systemProxyUrl: '',
  proxyErrorKeywords: '',
  proxyEmptyContentFailEnabled: false,
  payloadRules: '',
  codexUpstreamWebsocketEnabled: false,
  responsesCompactFallbackToResponsesEnabled: false,
  proxySessionChannelConcurrencyLimit: 2,
  proxySessionChannelQueueWaitMs: 1500,
  modelAvailabilityProbeEnabled: false,
}

function deriveServerValues(
  data: RuntimeSettings | undefined
): ProxyTransportFormValues | null {
  if (!data) {
    return null
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
  return {
    systemProxyUrl: asString(data.systemProxyUrl),
    proxyErrorKeywords: joinListField(splitListField(data.proxyErrorKeywords)),
    proxyEmptyContentFailEnabled: asBoolean(data.proxyEmptyContentFailEnabled),
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
  }
}

function proxyTransportToPayload(changed: Partial<ProxyTransportFormValues>) {
  const payload: Record<string, unknown> = {}
  if (changed.systemProxyUrl !== undefined) {
    payload.systemProxyUrl = changed.systemProxyUrl
  }
  if (changed.proxyErrorKeywords !== undefined) {
    payload.proxyErrorKeywords = splitListField(changed.proxyErrorKeywords)
  }
  if (changed.proxyEmptyContentFailEnabled !== undefined) {
    payload.proxyEmptyContentFailEnabled = changed.proxyEmptyContentFailEnabled
  }
  if (changed.payloadRules !== undefined) {
    payload.payloadRules = changed.payloadRules
      ? (JSON.parse(changed.payloadRules) as Record<string, unknown>)
      : null
  }
  if (changed.codexUpstreamWebsocketEnabled !== undefined) {
    payload.codexUpstreamWebsocketEnabled =
      changed.codexUpstreamWebsocketEnabled
  }
  if (changed.responsesCompactFallbackToResponsesEnabled !== undefined) {
    payload.responsesCompactFallbackToResponsesEnabled =
      changed.responsesCompactFallbackToResponsesEnabled
  }
  if (changed.proxySessionChannelConcurrencyLimit !== undefined) {
    payload.proxySessionChannelConcurrencyLimit =
      changed.proxySessionChannelConcurrencyLimit
  }
  if (changed.proxySessionChannelQueueWaitMs !== undefined) {
    payload.proxySessionChannelQueueWaitMs =
      changed.proxySessionChannelQueueWaitMs
  }
  if (changed.modelAvailabilityProbeEnabled !== undefined) {
    payload.modelAvailabilityProbeEnabled =
      changed.modelAvailabilityProbeEnabled
  }
  return payload
}

export function ProxyTransportSection() {
  const { t } = useTranslation()
  const { data, isLoading, isError, refetch } = useRuntimeSettings()
  const updateMutation = useUpdateRuntimeSettings()

  const serverValues = deriveServerValues(data)
  const { form, baseline, syncFromServer } =
    useSettingsForm<ProxyTransportFormValues>({
      schema: proxyTransportSchema,
      defaultValues: DEFAULT_VALUES,
      serverValues,
    })

  // POST /api/settings/system-proxy/test reports an unreachable proxy as
  // HTTP 200 + `{success:false, reachable:false, message}` — the verdict lives
  // in the envelope, not the status code. A failed probe must surface its real
  // outcome (and reason), never a "proxy reachable" celebration.
  const testProxyMutation = useMutation({
    mutationFn: async (proxyUrl: string) => {
      const result = await api.testSystemProxy({ proxyUrl })
      const probe = result as {
        success?: boolean
        reachable?: boolean
        latencyMs?: number
        message?: string
      } | null
      if (probe && probe.success === false) {
        throw new Error(
          probe.message ||
            t('settings.proxyModels.proxyTransport.toast.proxyFailed')
        )
      }
      return probe
    },
    onSuccess: (probe) => {
      const latency = probe?.latencyMs
      if (typeof latency === 'number' && latency > 0) {
        toast.success(
          t('settings.proxyModels.proxyTransport.toast.proxyOk', {
            ms: latency,
          })
        )
      } else {
        toast.success(
          t('settings.proxyModels.proxyTransport.toast.proxyOkGeneric')
        )
      }
    },
    onError: (error) => {
      const responseData = (
        error as {
          response?: { data?: { message?: string; error?: string } }
        } | null
      )?.response?.data
      const message =
        responseData?.message ||
        responseData?.error ||
        (error instanceof Error ? error.message : '') ||
        t('settings.proxyModels.proxyTransport.toast.proxyFailed')
      toast.error(message)
    },
  })

  function onSubmit(values: ProxyTransportFormValues) {
    const changed = collectChangedFields(values, baseline)
    if (!hasChanges(changed)) {
      toast.info(t('settings.common.noChanges'))
      return
    }
    updateMutation.mutate(proxyTransportToPayload(changed) as never, {
      onSuccess: () =>
        toast.success(t('settings.proxyModels.proxyTransport.toast.saved')),
      onError: () =>
        toast.error(t('settings.proxyModels.proxyTransport.toast.saveFailed')),
    })
  }

  if (isLoading) {
    return (
      <SettingsSectionCard
        title={t('settings.proxyModels.proxyTransport.title')}
        description={t('settings.proxyModels.proxyTransport.description')}
      >
        <p className='text-muted-foreground text-sm'>
          {t('settings.common.loading')}
        </p>
      </SettingsSectionCard>
    )
  }

  if (isError || !data) {
    return (
      <SettingsSectionError
        title={t('settings.proxyModels.proxyTransport.title')}
        onRetry={() => void refetch()}
      />
    )
  }

  const isDirty = form.formState.isDirty
  const systemProxyUrl = form.watch('systemProxyUrl')

  return (
    <SettingsSectionCard
      title={t('settings.proxyModels.proxyTransport.title')}
      description={t('settings.proxyModels.proxyTransport.description')}
      actions={
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={testProxyMutation.isPending || !systemProxyUrl}
          onClick={() => {
            if (systemProxyUrl) {
              testProxyMutation.mutate(systemProxyUrl)
            }
          }}
        >
          {testProxyMutation.isPending
            ? t('settings.common.testing')
            : t('settings.proxyModels.proxyTransport.testProxy')}
        </Button>
      }
    >
      <Form {...form}>
        <form
          id={FORM_ID}
          onSubmit={form.handleSubmit(onSubmit)}
          className='space-y-4'
        >
          <FormField
            control={form.control}
            name='systemProxyUrl'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t(
                    'settings.proxyModels.proxyTransport.fields.systemProxyUrl'
                  )}
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
            control={form.control}
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
            control={form.control}
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
                  {t(
                    'settings.proxyModels.proxyTransport.fields.payloadRulesHint'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <div className='space-y-4 rounded-lg border p-4'>
            <div className='space-y-1'>
              <h3 className='text-sm font-medium'>
                {t('settings.proxyModels.proxyTransport.codexUpstreamGroup')}
              </h3>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'settings.proxyModels.proxyTransport.codexUpstreamGroupHint'
                )}
              </p>
            </div>
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
                      'settings.proxyModels.proxyTransport.fields.codexUpstreamWebsocketEnabled'
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
                      'settings.proxyModels.proxyTransport.fields.responsesCompactFallbackEnabled'
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
                        'settings.proxyModels.proxyTransport.fields.proxySessionChannelConcurrencyLimit'
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
                        'settings.proxyModels.proxyTransport.fields.proxySessionChannelQueueWaitMs'
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

          <SettingsFormActions
            formId={FORM_ID}
            isDirty={isDirty}
            isPending={updateMutation.isPending}
            onReset={() =>
              syncFromServer(deriveServerValues(data) ?? DEFAULT_VALUES)
            }
          />
        </form>
      </Form>
      <FormNavigationGuard enabled={isDirty} />
    </SettingsSectionCard>
  )
}
