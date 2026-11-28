import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Spinner } from '@/components/ui/spinner'
import {
  upstreamPresetsApi,
  type UpstreamPreset,
} from '@/lib/api/upstream-presets'

import { UpstreamPresetPicker } from './upstream-preset-picker'

export function UpstreamCreatePresets({
  selectedId,
  pending,
  onSelect,
}: {
  selectedId: string | null
  pending: boolean
  onSelect: (preset: UpstreamPreset) => void
}) {
  const { t } = useTranslation()
  const presets = useQuery({
    queryKey: ['upstream-presets'],
    queryFn: upstreamPresetsApi.getUpstreamPresets,
  })
  return (
    <>
      <QueryErrorBanner
        error={presets.error}
        messageKey='channels.create.presetsFailed'
        onRetry={() => void presets.refetch()}
        isRetrying={presets.isFetching}
      />
      {presets.isPending && (
        <div role='status' className='flex items-center gap-2 text-sm'>
          <Spinner aria-hidden />
          {t('channels.create.loading')}
        </div>
      )}
      <UpstreamPresetPicker
        presets={presets.data?.items ?? []}
        selectedId={selectedId}
        disabled={pending}
        onSelect={onSelect}
      />
      {presets.data?.items.length === 0 && (
        <p className='text-muted-foreground text-sm'>
          {t('channels.create.noPresets')}
        </p>
      )}
    </>
  )
}
