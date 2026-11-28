import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import type { useReauthReplay } from '@/components/common/reauth-provider'
import { api, type BackupWebdavResponse } from '@/lib/api'
import { toast } from '@/lib/toast'

import { useSettingsForm } from '../../../hooks/use-settings-form'
import {
  collectChangedFields,
  hasChanges,
} from '../../../lib/collect-changed-fields'
import { scheduleFromLegacy, scheduleToCron } from '../../../lib/schedule'

const webdavSchema = z.object({
  enabled: z.boolean(),
  fileUrl: z.string().optional(),
  username: z.string().optional(),
  password: z.string().optional(),
  exportType: z.enum(['all', 'accounts', 'preferences']),
  autoSyncEnabled: z.boolean(),
  autoSyncSchedule: z.discriminatedUnion('kind', [
    z.object({
      version: z.literal(1),
      kind: z.literal('daily'),
      time: z.string(),
    }),
    z.object({
      version: z.literal(1),
      kind: z.literal('interval'),
      everyHours: z.number().int().min(1).max(24),
    }),
    z.object({
      version: z.literal(1),
      kind: z.literal('window'),
      windowStart: z.string(),
      windowEnd: z.string(),
    }),
    z.object({
      version: z.literal(1),
      kind: z.literal('custom'),
      cron: z.string(),
    }),
  ]),
})

export type WebdavFormValues = z.infer<typeof webdavSchema>

const DEFAULT_WEBDAV_VALUES: WebdavFormValues = {
  enabled: false,
  fileUrl: '',
  username: '',
  password: '',
  exportType: 'all',
  autoSyncEnabled: false,
  autoSyncSchedule: { version: 1, kind: 'interval', everyHours: 6 },
}

const webdavQueryKeys = {
  all: ['backup-webdav'] as const,
}

export function useWebdavConfig(
  handleReauth: ReturnType<typeof useReauthReplay>
) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const webdavQuery = useQuery<BackupWebdavResponse>({
    queryKey: webdavQueryKeys.all,
    queryFn: async () => api.getBackupWebdavConfig(),
    staleTime: 30 * 1000,
  })

  const config = webdavQuery.data?.config

  const serverValues = config
    ? {
        enabled: config.enabled,
        fileUrl: config.fileUrl,
        username: config.username,
        password: '',
        exportType: config.exportType,
        autoSyncEnabled: config.autoSyncEnabled,
        autoSyncSchedule: scheduleFromLegacy({ cron: config.autoSyncCron }),
      }
    : null
  const { form, baseline, syncFromServer } = useSettingsForm<WebdavFormValues>({
    schema: webdavSchema,
    defaultValues: DEFAULT_WEBDAV_VALUES,
    serverValues,
  })

  const saveWebdavMutation = useMutation({
    mutationFn: async (
      input: Partial<WebdavFormValues> & { confirmToken?: string }
    ) => {
      const { confirmToken, ...values } = input
      const payload: Parameters<typeof api.saveBackupWebdavConfig>[0] = {}
      if (values.enabled !== undefined) payload.enabled = values.enabled
      if (values.fileUrl !== undefined) payload.fileUrl = values.fileUrl ?? ''
      if (values.username !== undefined) {
        payload.username = values.username ?? ''
      }
      if (values.password) payload.password = values.password
      if (values.exportType !== undefined) {
        payload.exportType = values.exportType
      }
      if (values.autoSyncEnabled !== undefined) {
        payload.autoSyncEnabled = values.autoSyncEnabled
      }
      if (values.autoSyncSchedule !== undefined) {
        payload.autoSyncCron =
          scheduleToCron(values.autoSyncSchedule, config?.autoSyncCron) ??
          config?.autoSyncCron ??
          '0 */6 * * *'
      }
      return confirmToken
        ? api.saveBackupWebdavConfig(payload, confirmToken)
        : api.saveBackupWebdavConfig(payload)
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: webdavQueryKeys.all })
      toast.success(t('settings.content.importExport.toast.webdavSaved'))
    },
    onError: (error, input) => {
      if (
        handleReauth(
          error,
          t('reauth.operations.backupWebdavSave'),
          (confirmToken) =>
            saveWebdavMutation.mutateAsync({ ...input, confirmToken })
        )
      ) {
        return
      }
      toast.error(t('settings.content.importExport.toast.webdavSaveFailed'))
    },
  })

  function onWebdavSubmit(values: WebdavFormValues) {
    const changed = collectChangedFields(values, baseline)
    if (!hasChanges(changed)) {
      toast.info(t('settings.common.noChanges'))
      return
    }
    saveWebdavMutation.mutate(changed)
  }

  return {
    form,
    config,
    webdavQuery,
    onWebdavSubmit,
    savePending: saveWebdavMutation.isPending,
    reset: () => syncFromServer(serverValues ?? DEFAULT_WEBDAV_VALUES),
  }
}
