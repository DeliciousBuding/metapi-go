import { useTranslation } from 'react-i18next'

import { SectionCard } from '@/components/common/section-card'
import { SectionError } from '@/components/common/section-error'
import { Form } from '@/components/ui/form'
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
import { RoutingFallbackFields } from './routing-fallback-fields'
import {
  DEFAULT_VALUES,
  deriveServerValues,
  routingSchema,
  type RoutingFormValues,
} from './routing-form'
import { RoutingWeightFields } from './routing-weight-fields'

const FORM_ID = 'settings-proxy-models-routing-form'

export function RoutingSection() {
  const { t } = useTranslation()
  const { data, isLoading, isError, refetch } = useRuntimeSettings()
  const updateMutation = useUpdateRuntimeSettings()

  const serverValues = deriveServerValues(data)
  const { form, baseline, syncFromServer } = useSettingsForm<RoutingFormValues>(
    {
      schema: routingSchema,
      defaultValues: DEFAULT_VALUES,
      serverValues,
    }
  )

  function onSubmit(values: RoutingFormValues) {
    const changed = collectChangedFields(values, baseline)
    if (!hasChanges(changed)) {
      toast.info(t('settings.common.noChanges'))
      return
    }
    updateMutation.mutate(changed as never, {
      onSuccess: () =>
        toast.success(t('settings.proxyModels.routing.toast.saved')),
      onError: () =>
        toast.error(t('settings.proxyModels.routing.toast.saveFailed')),
    })
  }

  if (isLoading) {
    return (
      <SectionCard
        title={t('settings.proxyModels.routing.title')}
        description={t('settings.proxyModels.routing.description')}
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
        title={t('settings.proxyModels.routing.title')}
        messageKey='settings.common.loadFailed'
        onRetry={() => void refetch()}
      />
    )
  }

  const isDirty = form.formState.isDirty

  return (
    <SectionCard
      title={t('settings.proxyModels.routing.title')}
      description={t('settings.proxyModels.routing.description')}
    >
      <Form {...form}>
        <form
          id={FORM_ID}
          onSubmit={form.handleSubmit(onSubmit)}
          className='space-y-4'
        >
          <RoutingFallbackFields />
          <RoutingWeightFields />
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
