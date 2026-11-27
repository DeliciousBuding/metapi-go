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

import { SettingsSubsection } from '../../../components/settings-subsection'
import { asString, type RuntimeSettings } from '../../../lib/runtime-settings'
import {
  NotificationToggle,
  NotificationText,
  NotificationSecret,
} from './notifications-fields'
import type { NotifyFormValues } from './notifications-form-values'

function TelegramChannel({ data }: { data: RuntimeSettings }) {
  const { t } = useTranslation()
  return (
    <SettingsSubsection
      variant='panel'
      title={t('settings.content.notifications.channels.telegram')}
    >
      <NotificationToggle name='telegramEnabled' />
      <NotificationText name='telegramChatId' />
      <NotificationText name='telegramMessageThreadId' />
      <NotificationText
        name='telegramApiBaseUrl'
        url
        placeholder='https://api.telegram.org'
      />
      <NotificationToggle name='telegramUseSystemProxy' />
      <NotificationSecret
        name='telegramBotToken'
        masked={asString(data.telegramBotTokenMasked || data.telegramBotToken)}
      />
    </SettingsSubsection>
  )
}

function SmtpChannel({ data }: { data: RuntimeSettings }) {
  const { t } = useTranslation()
  const { control } = useFormContext<NotifyFormValues>()
  return (
    <SettingsSubsection
      variant='panel'
      title={t('settings.content.notifications.channels.smtp')}
    >
      <NotificationToggle name='smtpEnabled' />
      <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
        <NotificationText name='smtpHost' />
        <FormField
          control={control}
          name='smtpPort'
          render={({ field }) => (
            <FormItem>
              <FormLabel>
                {t('settings.content.notifications.fields.smtpPort')}
              </FormLabel>
              <FormControl>
                <Input {...field} value={field.value ?? ''} type='number' />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <NotificationText name='smtpUser' />
        <NotificationSecret
          name='smtpPass'
          masked={asString(data.smtpPassMasked)}
        />
        <NotificationText name='smtpFrom' />
        <NotificationText name='smtpTo' />
      </div>
      <NotificationToggle name='smtpSecure' />
    </SettingsSubsection>
  )
}

export function NotificationChannels({ data }: { data: RuntimeSettings }) {
  const { t } = useTranslation()
  return (
    <>
      <SettingsSubsection
        variant='panel'
        title={t('settings.content.notifications.channels.webhook')}
      >
        <NotificationToggle name='webhookEnabled' />
        <NotificationText
          name='webhookUrl'
          url
          placeholder='https://your-webhook-url'
        />
      </SettingsSubsection>

      <SettingsSubsection
        variant='panel'
        title={t('settings.content.notifications.channels.bark')}
      >
        <NotificationToggle name='barkEnabled' />
        <NotificationText
          name='barkUrl'
          url
          placeholder='https://api.day.app/your_key'
        />
      </SettingsSubsection>

      <SettingsSubsection
        variant='panel'
        title={t('settings.content.notifications.channels.serverChan')}
      >
        <NotificationToggle name='serverChanEnabled' />
        <NotificationSecret
          name='serverChanKey'
          masked={asString(data.serverChanKeyMasked)}
        />
      </SettingsSubsection>

      <TelegramChannel data={data} />

      <SmtpChannel data={data} />

      <SettingsSubsection
        variant='panel'
        title={t('settings.content.notifications.channels.feishu')}
      >
        <NotificationToggle name='feishuEnabled' />
        <NotificationText name='feishuWebhook' url />
        <NotificationSecret
          name='feishuSecret'
          masked={asString(data.feishuSecretMasked)}
        />
      </SettingsSubsection>

      <SettingsSubsection
        variant='panel'
        title={t('settings.content.notifications.channels.dingtalk')}
      >
        <NotificationToggle name='dingtalkEnabled' />
        <NotificationText name='dingtalkWebhook' url />
        <NotificationSecret
          name='dingtalkSecret'
          masked={asString(data.dingtalkSecretMasked)}
        />
      </SettingsSubsection>

      <SettingsSubsection
        variant='panel'
        title={t('settings.content.notifications.channels.wecom')}
      >
        <NotificationToggle name='wecomEnabled' />
        <NotificationText name='wecomWebhook' url />
      </SettingsSubsection>

      <SettingsSubsection
        variant='panel'
        title={t('settings.content.notifications.channels.ntfy')}
      >
        <NotificationToggle name='ntfyEnabled' />
        <NotificationText name='ntfyUrl' url placeholder='https://ntfy.sh' />
        <NotificationText name='ntfyTopic' />
        <NotificationSecret
          name='ntfyToken'
          masked={asString(data.ntfyTokenMasked)}
        />
      </SettingsSubsection>
    </>
  )
}
