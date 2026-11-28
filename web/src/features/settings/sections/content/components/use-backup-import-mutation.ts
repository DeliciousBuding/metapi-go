import { useMutation, useQueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { useTranslation } from 'react-i18next'

import { useReauthReplay } from '@/components/common/reauth-provider'
import { api } from '@/lib/api'
import {
  stripBackupBOM,
  type ExternalBackupSource,
} from '@/lib/helpers/external-backup-source'
import { toast } from '@/lib/toast'

export function useBackupImportMutation(
  onImported: () => void,
  refreshPreview: () => Promise<void>
) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const handleReauth = useReauthReplay()
  const importMutation = useMutation({
    retry: false,
    mutationFn: async (input: {
      raw: string
      originKey?: string
      source?: ExternalBackupSource
      channelsOnly?: boolean
      replaceOrigin?: boolean
      replacementRevision?: string
      magpiePassphrase?: string
      confirmToken?: string
    }) => {
      const { raw, originKey, source, channelsOnly, replaceOrigin } = input
      const data = JSON.parse(stripBackupBOM(raw)) as unknown
      const octopus = source === 'octopus-v5'
      const originKeyArg = source ? originKey : undefined
      const octopusMode =
        octopus && channelsOnly ? ('channels-only' as const) : undefined
      const replaceOctopus = octopus && replaceOrigin
      const replaceAxonHub = source === 'axonhub-v1.4' && replaceOrigin
      const replaceMagpie = source === 'magpie-backup' && replaceOrigin
      return input.confirmToken
        ? api.importBackup(data, {
            externalOriginKey: originKeyArg,
            octopusImportMode: octopusMode,
            replaceOctopusOrigin: replaceOctopus,
            replaceAxonHubOrigin: replaceAxonHub,
            replaceMagpieOrigin: replaceMagpie,
            replacementRevision: input.replacementRevision,
            magpiePassphrase: input.magpiePassphrase,
            confirmToken: input.confirmToken,
          })
        : api.importBackup(data, {
            externalOriginKey: originKeyArg,
            octopusImportMode: octopusMode,
            replaceOctopusOrigin: replaceOctopus,
            replaceAxonHubOrigin: replaceAxonHub,
            replaceMagpieOrigin: replaceMagpie,
            replacementRevision: input.replacementRevision,
            magpiePassphrase: input.magpiePassphrase,
          })
    },
    onSuccess: () => {
      toast.success(t('settings.content.importExport.toast.imported'))
      onImported()
      // A backup restores rows across the schema, including settings and routes.
      void queryClient.invalidateQueries()
    },
    onError: (error: unknown, input) => {
      if (isAxiosError(error) && error.response?.status === 409) {
        toast.error(t('settings.content.importExport.toast.previewChanged'))
        void refreshPreview()
        return
      }
      if (
        handleReauth(
          error,
          t('reauth.operations.backupImport'),
          (confirmToken) =>
            importMutation.mutateAsync({ ...input, confirmToken })
        )
      ) {
        return
      }
      toast.error(t('settings.content.importExport.toast.importFailed'))
    },
  })
  return importMutation
}
