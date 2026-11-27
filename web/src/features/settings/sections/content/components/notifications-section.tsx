import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { SectionCard } from '@/components/common/section-card'
import { SectionError } from '@/components/common/section-error'
import { Button } from '@/components/ui/button'
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
import { NotificationChannels } from './notifications-channels'
import { NotificationToggle } from './notifications-fields'
import {
  notifySchema,
  DEFAULT_VALUES,
  deriveServerValues,
  notificationsToPayload,
  type NotifyFormValues,
} from './notifications-form-values'

const FORM_ID = 'settings-content-notifications-form'

export function NotificationsSection() {
  const { t } = useTranslation()
  const { data, isLoading, isError, refetch } = useRuntimeSettings()
  const updateMutation = useUpdateRuntimeSettings()

  const serverValues = deriveServerValues(data)
  const { form, baseline, syncFromServer } = useSettingsForm<NotifyFormValues>({
    schema: notifySchema,
    defaultValues: DEFAULT_VALUES,
    serverValues,
  })

  const testMutation = useMutation({
    mutationFn: async () => api.testNotification(),
    onSuccess: () =>
      toast.success(t('settings.content.notifications.toast.testSent')),
    onError: () =>
      toast.error(t('settings.content.notifications.toast.testFailed')),
  })

  function onSubmit(values: NotifyFormValues) {
    const changed = collectChangedFields(values, baseline)
    if (!hasChanges(changed)) {
      toast.info(t('settings.common.noChanges'))
      return
    }
    updateMutation.mutate(notificationsToPayload(changed, baseline) as never, {
      onSuccess: () =>
        toast.success(t('settings.content.notifications.toast.saved')),
      onError: () =>
        toast.error(t('settings.content.notifications.toast.saveFailed')),
    })
  }

  if (isLoading) {
    return (
      <SectionCard
        title={t('settings.content.notifications.title')}
        description={t('settings.content.notifications.description')}
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
        title={t('settings.content.notifications.title')}
        messageKey='settings.common.loadFailed'
        onRetry={() => void refetch()}
      />
    )
  }

  const isDirty = form.formState.isDirty

  return (
    <SectionCard
      title={t('settings.content.notifications.title')}
      description={t('settings.content.notifications.description')}
      hideHeaderCopy
      actions={
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={testMutation.isPending}
          onClick={() => testMutation.mutate()}
        >
          {testMutation.isPending
            ? t('settings.common.testing')
            : t('settings.content.notifications.test')}
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
            name='notifyCooldownSec'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('settings.content.notifications.fields.notifyCooldownSec')}
                </FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    value={field.value ?? ''}
                    type='number'
                    min={0}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'settings.content.notifications.fields.notifyCooldownSecHint'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <NotificationChannels data={data} />

          <div className='space-y-3 rounded-lg border p-4'>
            <h3 className='text-sm font-medium'>
              {t('settings.content.notifications.muteGroup')}
            </h3>
            <NotificationToggle name='muteTokenExpired' />
            <NotificationToggle name='muteLowBalance' />
            <NotificationToggle name='muteProxyAllFailed' />
          </div>

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
