// metapi-go/features/settings/sections/content/components — notification
// channels section. All channels (webhook/bark/serverchan/telegram/smtp/
// feishu/dingtalk/wecom/ntfy) in one form, plus the per-task mute toggles
// and the "send test notification" action. Secrets are only sent when the
// user types a fresh value; blank = keep the stored secret, masked values
// are never echoed back.

import { useMutation } from '@tanstack/react-query'
import type {
  ControllerRenderProps,
  FieldPath,
  FieldValues,
} from 'react-hook-form'
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
  useRuntimeSettings,
  useUpdateRuntimeSettings,
  type RuntimeSettings,
} from '../../../lib/runtime-settings'

const FORM_ID = 'settings-content-notifications-form'

const notifySchema = z.object({
  notifyCooldownSec: z.coerce.number().int().min(0),
  webhookEnabled: z.boolean(),
  webhookUrl: z.string().optional(),
  barkEnabled: z.boolean(),
  barkUrl: z.string().optional(),
  serverChanEnabled: z.boolean(),
  serverChanKey: z.string().optional(),
  telegramEnabled: z.boolean(),
  telegramUseSystemProxy: z.boolean(),
  telegramApiBaseUrl: z.string().optional(),
  telegramBotToken: z.string().optional(),
  telegramChatId: z.string().optional(),
  telegramMessageThreadId: z.string().optional(),
  smtpEnabled: z.boolean(),
  smtpHost: z.string().optional(),
  smtpPort: z.coerce.number().int(),
  smtpSecure: z.boolean(),
  smtpUser: z.string().optional(),
  smtpPass: z.string().optional(),
  smtpFrom: z.string().optional(),
  smtpTo: z.string().optional(),
  feishuEnabled: z.boolean(),
  feishuWebhook: z.string().optional(),
  feishuSecret: z.string().optional(),
  dingtalkEnabled: z.boolean(),
  dingtalkWebhook: z.string().optional(),
  dingtalkSecret: z.string().optional(),
  wecomEnabled: z.boolean(),
  wecomWebhook: z.string().optional(),
  ntfyEnabled: z.boolean(),
  ntfyUrl: z.string().optional(),
  ntfyTopic: z.string().optional(),
  ntfyToken: z.string().optional(),
  muteTokenExpired: z.boolean(),
  muteLowBalance: z.boolean(),
  muteProxyAllFailed: z.boolean(),
})

type NotifyFormValues = z.infer<typeof notifySchema>

const SECRET_FIELDS = [
  'serverChanKey',
  'telegramBotToken',
  'smtpPass',
  'feishuSecret',
  'dingtalkSecret',
  'ntfyToken',
] as const

type SecretFieldName = (typeof SECRET_FIELDS)[number]

const DEFAULT_VALUES: NotifyFormValues = {
  notifyCooldownSec: 300,
  webhookEnabled: false,
  webhookUrl: '',
  barkEnabled: false,
  barkUrl: '',
  serverChanEnabled: false,
  serverChanKey: '',
  telegramEnabled: false,
  telegramUseSystemProxy: false,
  telegramApiBaseUrl: 'https://api.telegram.org',
  telegramBotToken: '',
  telegramChatId: '',
  telegramMessageThreadId: '',
  smtpEnabled: false,
  smtpHost: '',
  smtpPort: 587,
  smtpSecure: false,
  smtpUser: '',
  smtpPass: '',
  smtpFrom: '',
  smtpTo: '',
  feishuEnabled: false,
  feishuWebhook: '',
  feishuSecret: '',
  dingtalkEnabled: false,
  dingtalkWebhook: '',
  dingtalkSecret: '',
  wecomEnabled: false,
  wecomWebhook: '',
  ntfyEnabled: false,
  ntfyUrl: 'https://ntfy.sh',
  ntfyTopic: '',
  ntfyToken: '',
  muteTokenExpired: false,
  muteLowBalance: false,
  muteProxyAllFailed: false,
}

function deriveServerValues(
  data: RuntimeSettings | undefined
): NotifyFormValues | null {
  if (!data) {
    return null
  }
  const toggles = (data.notifyTaskToggles ?? {}) as Record<string, unknown>
  return {
    notifyCooldownSec: asNumber(data.notifyCooldownSec) ?? 300,
    webhookEnabled: asBoolean(data.webhookEnabled),
    webhookUrl: asString(data.webhookUrl),
    barkEnabled: asBoolean(data.barkEnabled),
    barkUrl: asString(data.barkUrl),
    serverChanEnabled: asBoolean(data.serverChanEnabled),
    serverChanKey: '',
    telegramEnabled: asBoolean(data.telegramEnabled),
    telegramUseSystemProxy: asBoolean(data.telegramUseSystemProxy),
    telegramApiBaseUrl:
      asString(data.telegramApiBaseUrl) || 'https://api.telegram.org',
    telegramBotToken: '',
    telegramChatId: asString(data.telegramChatId),
    telegramMessageThreadId: asString(data.telegramMessageThreadId),
    smtpEnabled: asBoolean(data.smtpEnabled),
    smtpHost: asString(data.smtpHost),
    smtpPort: asNumber(data.smtpPort) ?? 587,
    smtpSecure: asBoolean(data.smtpSecure),
    smtpUser: asString(data.smtpUser),
    smtpPass: '',
    smtpFrom: asString(data.smtpFrom),
    smtpTo: asString(data.smtpTo),
    feishuEnabled: asBoolean(data.feishuEnabled),
    feishuWebhook: asString(data.feishuWebhook),
    feishuSecret: '',
    dingtalkEnabled: asBoolean(data.dingtalkEnabled),
    dingtalkWebhook: asString(data.dingtalkWebhook),
    dingtalkSecret: '',
    wecomEnabled: asBoolean(data.wecomEnabled),
    wecomWebhook: asString(data.wecomWebhook),
    ntfyEnabled: asBoolean(data.ntfyEnabled),
    ntfyUrl: asString(data.ntfyUrl) || 'https://ntfy.sh',
    ntfyTopic: asString(data.ntfyTopic),
    ntfyToken: '',
    muteTokenExpired: asBoolean(toggles.token_expired),
    muteLowBalance: asBoolean(toggles.low_balance),
    muteProxyAllFailed: asBoolean(toggles.proxy_all_failed),
  }
}

function notificationsToPayload(
  changed: Partial<NotifyFormValues>,
  baseline: NotifyFormValues | null
): Record<string, unknown> {
  const payload: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(changed)) {
    if (SECRET_FIELDS.includes(key as SecretFieldName)) {
      // Blank secret = keep stored; never submit a masked value.
      if (value) {
        payload[key] = value
      }
      continue
    }
    payload[key] = value
  }
  // Mute toggles are stored as notifyTaskToggles; submit the whole map when
  // any mute field changed.
  const muteChanged =
    changed.muteTokenExpired !== undefined ||
    changed.muteLowBalance !== undefined ||
    changed.muteProxyAllFailed !== undefined
  if (muteChanged) {
    const current = { ...(baseline ?? DEFAULT_VALUES), ...changed }
    payload.notifyTaskToggles = {
      token_expired: current.muteTokenExpired,
      low_balance: current.muteLowBalance,
      proxy_all_failed: current.muteProxyAllFailed,
    }
  }
  delete payload.muteTokenExpired
  delete payload.muteLowBalance
  delete payload.muteProxyAllFailed
  return payload
}

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
    const changed = collectChangedFields(
      values as unknown as Record<string, unknown>,
      baseline as unknown as Record<string, unknown> | null
    ) as Partial<NotifyFormValues>
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
      <SettingsSectionCard
        title={t('settings.content.notifications.title')}
        description={t('settings.content.notifications.description')}
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
        title={t('settings.content.notifications.title')}
        onRetry={() => void refetch()}
      />
    )
  }

  const isDirty = form.formState.isDirty

  return (
    <SettingsSectionCard
      title={t('settings.content.notifications.title')}
      description={t('settings.content.notifications.description')}
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

          <ChannelGroup
            title={t('settings.content.notifications.channels.webhook')}
          >
            <FormField
              control={form.control}
              name='webhookEnabled'
              render={({ field }) => (
                <ToggleField
                  label='settings.content.notifications.fields.webhookEnabled'
                  field={field}
                />
              )}
            />
            <FormField
              control={form.control}
              name='webhookUrl'
              render={({ field }) => (
                <UrlField
                  label='settings.content.notifications.fields.webhookUrl'
                  field={field}
                  placeholder='https://your-webhook-url'
                />
              )}
            />
          </ChannelGroup>

          <ChannelGroup
            title={t('settings.content.notifications.channels.bark')}
          >
            <FormField
              control={form.control}
              name='barkEnabled'
              render={({ field }) => (
                <ToggleField
                  label='settings.content.notifications.fields.barkEnabled'
                  field={field}
                />
              )}
            />
            <FormField
              control={form.control}
              name='barkUrl'
              render={({ field }) => (
                <UrlField
                  label='settings.content.notifications.fields.barkUrl'
                  field={field}
                  placeholder='https://api.day.app/your_key'
                />
              )}
            />
          </ChannelGroup>

          <ChannelGroup
            title={t('settings.content.notifications.channels.serverChan')}
          >
            <FormField
              control={form.control}
              name='serverChanEnabled'
              render={({ field }) => (
                <ToggleField
                  label='settings.content.notifications.fields.serverChanEnabled'
                  field={field}
                />
              )}
            />
            <FormField
              control={form.control}
              name='serverChanKey'
              render={({ field }) => (
                <SecretField
                  label='settings.content.notifications.fields.serverChanKey'
                  masked={asString(data.serverChanKeyMasked)}
                  field={field}
                />
              )}
            />
          </ChannelGroup>

          <ChannelGroup
            title={t('settings.content.notifications.channels.telegram')}
          >
            <FormField
              control={form.control}
              name='telegramEnabled'
              render={({ field }) => (
                <ToggleField
                  label='settings.content.notifications.fields.telegramEnabled'
                  field={field}
                />
              )}
            />
            <FormField
              control={form.control}
              name='telegramChatId'
              render={({ field }) => (
                <TextField
                  label='settings.content.notifications.fields.telegramChatId'
                  field={field}
                />
              )}
            />
            <FormField
              control={form.control}
              name='telegramMessageThreadId'
              render={({ field }) => (
                <TextField
                  label='settings.content.notifications.fields.telegramMessageThreadId'
                  field={field}
                />
              )}
            />
            <FormField
              control={form.control}
              name='telegramApiBaseUrl'
              render={({ field }) => (
                <UrlField
                  label='settings.content.notifications.fields.telegramApiBaseUrl'
                  field={field}
                  placeholder='https://api.telegram.org'
                />
              )}
            />
            <FormField
              control={form.control}
              name='telegramUseSystemProxy'
              render={({ field }) => (
                <ToggleField
                  label='settings.content.notifications.fields.telegramUseSystemProxy'
                  field={field}
                />
              )}
            />
            <FormField
              control={form.control}
              name='telegramBotToken'
              render={({ field }) => (
                <SecretField
                  label='settings.content.notifications.fields.telegramBotToken'
                  masked={asString(
                    data.telegramBotTokenMasked || data.telegramBotToken
                  )}
                  field={field}
                />
              )}
            />
          </ChannelGroup>

          <ChannelGroup
            title={t('settings.content.notifications.channels.smtp')}
          >
            <FormField
              control={form.control}
              name='smtpEnabled'
              render={({ field }) => (
                <ToggleField
                  label='settings.content.notifications.fields.smtpEnabled'
                  field={field}
                />
              )}
            />
            <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='smtpHost'
                render={({ field }) => (
                  <TextField
                    label='settings.content.notifications.fields.smtpHost'
                    field={field}
                  />
                )}
              />
              <FormField
                control={form.control}
                name='smtpPort'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {t('settings.content.notifications.fields.smtpPort')}
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? ''}
                        type='number'
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='smtpUser'
                render={({ field }) => (
                  <TextField
                    label='settings.content.notifications.fields.smtpUser'
                    field={field}
                  />
                )}
              />
              <FormField
                control={form.control}
                name='smtpPass'
                render={({ field }) => (
                  <SecretField
                    label='settings.content.notifications.fields.smtpPass'
                    masked={asString(data.smtpPassMasked)}
                    field={field}
                  />
                )}
              />
              <FormField
                control={form.control}
                name='smtpFrom'
                render={({ field }) => (
                  <TextField
                    label='settings.content.notifications.fields.smtpFrom'
                    field={field}
                  />
                )}
              />
              <FormField
                control={form.control}
                name='smtpTo'
                render={({ field }) => (
                  <TextField
                    label='settings.content.notifications.fields.smtpTo'
                    field={field}
                  />
                )}
              />
            </div>
            <FormField
              control={form.control}
              name='smtpSecure'
              render={({ field }) => (
                <ToggleField
                  label='settings.content.notifications.fields.smtpSecure'
                  field={field}
                />
              )}
            />
          </ChannelGroup>

          <ChannelGroup
            title={t('settings.content.notifications.channels.feishu')}
          >
            <FormField
              control={form.control}
              name='feishuEnabled'
              render={({ field }) => (
                <ToggleField
                  label='settings.content.notifications.fields.feishuEnabled'
                  field={field}
                />
              )}
            />
            <FormField
              control={form.control}
              name='feishuWebhook'
              render={({ field }) => (
                <UrlField
                  label='settings.content.notifications.fields.feishuWebhook'
                  field={field}
                />
              )}
            />
            <FormField
              control={form.control}
              name='feishuSecret'
              render={({ field }) => (
                <SecretField
                  label='settings.content.notifications.fields.feishuSecret'
                  masked={asString(data.feishuSecretMasked)}
                  field={field}
                />
              )}
            />
          </ChannelGroup>

          <ChannelGroup
            title={t('settings.content.notifications.channels.dingtalk')}
          >
            <FormField
              control={form.control}
              name='dingtalkEnabled'
              render={({ field }) => (
                <ToggleField
                  label='settings.content.notifications.fields.dingtalkEnabled'
                  field={field}
                />
              )}
            />
            <FormField
              control={form.control}
              name='dingtalkWebhook'
              render={({ field }) => (
                <UrlField
                  label='settings.content.notifications.fields.dingtalkWebhook'
                  field={field}
                />
              )}
            />
            <FormField
              control={form.control}
              name='dingtalkSecret'
              render={({ field }) => (
                <SecretField
                  label='settings.content.notifications.fields.dingtalkSecret'
                  masked={asString(data.dingtalkSecretMasked)}
                  field={field}
                />
              )}
            />
          </ChannelGroup>

          <ChannelGroup
            title={t('settings.content.notifications.channels.wecom')}
          >
            <FormField
              control={form.control}
              name='wecomEnabled'
              render={({ field }) => (
                <ToggleField
                  label='settings.content.notifications.fields.wecomEnabled'
                  field={field}
                />
              )}
            />
            <FormField
              control={form.control}
              name='wecomWebhook'
              render={({ field }) => (
                <UrlField
                  label='settings.content.notifications.fields.wecomWebhook'
                  field={field}
                />
              )}
            />
          </ChannelGroup>

          <ChannelGroup
            title={t('settings.content.notifications.channels.ntfy')}
          >
            <FormField
              control={form.control}
              name='ntfyEnabled'
              render={({ field }) => (
                <ToggleField
                  label='settings.content.notifications.fields.ntfyEnabled'
                  field={field}
                />
              )}
            />
            <FormField
              control={form.control}
              name='ntfyUrl'
              render={({ field }) => (
                <UrlField
                  label='settings.content.notifications.fields.ntfyUrl'
                  field={field}
                  placeholder='https://ntfy.sh'
                />
              )}
            />
            <FormField
              control={form.control}
              name='ntfyTopic'
              render={({ field }) => (
                <TextField
                  label='settings.content.notifications.fields.ntfyTopic'
                  field={field}
                />
              )}
            />
            <FormField
              control={form.control}
              name='ntfyToken'
              render={({ field }) => (
                <SecretField
                  label='settings.content.notifications.fields.ntfyToken'
                  masked={asString(data.ntfyTokenMasked)}
                  field={field}
                />
              )}
            />
          </ChannelGroup>

          <div className='space-y-3 rounded-lg border p-4'>
            <h4 className='text-sm font-medium'>
              {t('settings.content.notifications.muteGroup')}
            </h4>
            <FormField
              control={form.control}
              name='muteTokenExpired'
              render={({ field }) => (
                <MuteField
                  label='settings.content.notifications.fields.muteTokenExpired'
                  field={field}
                />
              )}
            />
            <FormField
              control={form.control}
              name='muteLowBalance'
              render={({ field }) => (
                <MuteField
                  label='settings.content.notifications.fields.muteLowBalance'
                  field={field}
                />
              )}
            />
            <FormField
              control={form.control}
              name='muteProxyAllFailed'
              render={({ field }) => (
                <MuteField
                  label='settings.content.notifications.fields.muteProxyAllFailed'
                  field={field}
                />
              )}
            />
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
    </SettingsSectionCard>
  )
}

// --- inline sub-components (kept inside this file because they are tightly
// coupled to the form's ControllerProps shape and the i18n key namespace) ---

type FieldProps<TValues extends FieldValues> = {
  field: ControllerRenderProps<TValues, FieldPath<TValues>>
  label: string
}

function ChannelGroup({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <div className='space-y-3 rounded-lg border p-4'>
      <h4 className='text-sm font-medium'>{title}</h4>
      {children}
    </div>
  )
}

function ToggleField<TValues extends FieldValues>({
  label,
  field,
}: FieldProps<TValues>) {
  const { t } = useTranslation()
  return (
    <FormItem className='flex flex-row items-center gap-3'>
      <FormControl>
        <Checkbox
          checked={Boolean(field.value)}
          onCheckedChange={field.onChange}
        />
      </FormControl>
      <FormLabel className='cursor-pointer'>{t(label)}</FormLabel>
    </FormItem>
  )
}

function TextField<TValues extends FieldValues>({
  label,
  field,
}: FieldProps<TValues>) {
  const { t } = useTranslation()
  return (
    <FormItem>
      <FormLabel>{t(label)}</FormLabel>
      <FormControl>
        <Input {...field} value={field.value ?? ''} />
      </FormControl>
      <FormMessage />
    </FormItem>
  )
}

function UrlField<TValues extends FieldValues>({
  label,
  field,
  placeholder,
}: FieldProps<TValues> & { placeholder?: string }) {
  const { t } = useTranslation()
  return (
    <FormItem>
      <FormLabel>{t(label)}</FormLabel>
      <FormControl>
        <Input
          {...field}
          value={field.value ?? ''}
          placeholder={placeholder}
          className='font-mono'
        />
      </FormControl>
      <FormMessage />
    </FormItem>
  )
}

function SecretField<TValues extends FieldValues>({
  label,
  field,
  masked,
}: FieldProps<TValues> & { masked: string }) {
  const { t } = useTranslation()
  return (
    <FormItem>
      <FormLabel>{t(label)}</FormLabel>
      <FormControl>
        <Input
          {...field}
          value={field.value ?? ''}
          type='password'
          placeholder={t('settings.content.notifications.secretPlaceholder')}
        />
      </FormControl>
      {masked ? (
        <FormDescription>
          {t('settings.content.notifications.masked', { masked })}
        </FormDescription>
      ) : null}
      <FormMessage />
    </FormItem>
  )
}

function MuteField<TValues extends FieldValues>({
  label,
  field,
}: FieldProps<TValues>) {
  const { t } = useTranslation()
  return (
    <FormItem className='flex flex-row items-center gap-3'>
      <FormControl>
        <Checkbox
          checked={Boolean(field.value)}
          onCheckedChange={field.onChange}
        />
      </FormControl>
      <FormLabel className='cursor-pointer'>{t(label)}</FormLabel>
    </FormItem>
  )
}
