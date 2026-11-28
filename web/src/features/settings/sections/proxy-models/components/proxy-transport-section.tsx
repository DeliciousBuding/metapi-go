import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { SectionCard } from '@/components/common/section-card'
import { SectionError } from '@/components/common/section-error'
import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import { api } from '@/lib/api'
import { toast } from '@/lib/toast'

import { FormNavigationGuard } from '../../../components/form-navigation-guard'
import { SettingsFormActions } from '../../../components/settings-form-actions'
import { useSettingsForm } from '../../../hooks/use-settings-form'
import {
  collectChangedFields,
  hasChanges,
} from '../../../lib/collect-changed-fields'
import {
  useRuntimeSettings,
  useUpdateRuntimeSettings,
} from '../../../lib/runtime-settings'
import {
  ProxyRequestFields,
  ProxyAvailabilityFields,
} from './proxy-transport-fields'
import {
  DEFAULT_VALUES,
  deriveServerValues,
  proxyTransportSchema,
  proxyTransportToPayload,
  type ProxyTransportFormValues,
} from './proxy-transport-schema'

const FORM_ID = 'settings-proxy-models-proxy-transport-form'

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
      <SectionCard
        title={t('settings.proxyModels.proxyTransport.title')}
        description={t('settings.proxyModels.proxyTransport.description')}
      >
        <p className='text-muted-foreground text-sm'>
          {t('settings.common.loading')}
        </p>
      </SectionCard>
    )
  }

  if (isError || !data) {
    return (
      <SectionError
        title={t('settings.proxyModels.proxyTransport.title')}
        messageKey='settings.common.loadFailed'
        onRetry={() => void refetch()}
      />
    )
  }

  const isDirty = form.formState.isDirty
  const systemProxyUrl = form.watch('systemProxyUrl')

  return (
    <SectionCard
      title={t('settings.proxyModels.proxyTransport.title')}
      description={t('settings.proxyModels.proxyTransport.description')}
      hideHeaderCopy
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
          <ProxyRequestFields control={form.control} />
          <ProxyAvailabilityFields control={form.control} />
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
    </SectionCard>
  )
}
