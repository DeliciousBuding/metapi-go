import { CircleCheck, TriangleAlert, Layers3 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import type { UpstreamDeletionPreview } from '@/lib/api/upstream-lifecycle'

import { BackupImportExternalSections } from './backup-import-external-sections'
import { getBackupImportReview } from './backup-import-review'

export type BackupImportTablePlan = Record<
  string,
  { rows: number; toInsert: number; duplicates: number; skippedRows: number }
>

export type OctopusV5Preview = {
  source: string
  originKey: string
  sections: Record<string, number>
  notImported?: Record<string, number>
  removals?: Record<string, number>
  removalImpact?: UpstreamDeletionPreview
  adaptations?: string[]
  blocking?: string[]
}

type AxonHubSkippedChannel = {
  sourceId: number
  type: string
  reasons: string[]
}

export type AxonHubV14Preview = {
  source: string
  originKey: string
  version: string
  sections: Record<string, number>
  routable: Record<string, number>
  notImported?: Record<string, number>
  skippedChannels?: AxonHubSkippedChannel[]
  residuals?: string[]
  removals?: Record<string, number>
  removalImpact?: UpstreamDeletionPreview
  blocking?: string[]
}

type MagpieSkippedProvider = {
  sourceId: string
  type: string
  reasons: string[]
}

type MagpieSkippedGroup = {
  sourceId: string
  reasons: string[]
}

export type MagpieBackupPreview = {
  source: string
  originKey: string
  version: string
  sections: Record<string, number>
  routable: Record<string, number>
  notImported?: Record<string, number>
  skippedProviders?: MagpieSkippedProvider[]
  skippedGroups?: MagpieSkippedGroup[]
  residuals?: string[]
  removals?: Record<string, number>
  removalImpact?: UpstreamDeletionPreview
  blocking?: string[]
}

export type BackupImportPreview =
  | { kind: 'tables'; tables: BackupImportTablePlan }
  | { kind: 'octopus'; data: OctopusV5Preview }
  | { kind: 'axonhub'; data: AxonHubV14Preview }
  | { kind: 'magpie'; data: MagpieBackupPreview }

type Props = {
  preview: BackupImportPreview | null
  acknowledged: boolean
  onAcknowledge: (checked: boolean) => void
}

export function BackupImportPreviewPanel(props: Props) {
  const { t } = useTranslation()
  const preview = props.preview
  if (!preview) return null
  const review = getBackupImportReview(preview)
  const { external, status, variant } = review
  return (
    <section
      className='overflow-hidden rounded-xl border'
      aria-label={t('settings.content.importExport.importPreviewTitle')}
    >
      <div className='bg-muted/40 flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3'>
        <h3 className='flex items-center gap-2 text-sm font-semibold'>
          <Layers3 className='text-primary size-4' aria-hidden='true' />
          {t('settings.content.importExport.importPreviewTitle')}
        </h3>
        <Badge variant={variant}>
          {status === 'ready' ? (
            <CircleCheck aria-hidden='true' />
          ) : (
            <TriangleAlert aria-hidden='true' />
          )}
          {t(`settings.content.importExport.design.${status}`)}
        </Badge>
      </div>
      <div className='space-y-4 p-4'>
        {external ? (
          <BackupImportExternalSections
            preview={preview}
            review={review}
            acknowledged={props.acknowledged}
            onAcknowledge={props.onAcknowledge}
          />
        ) : (
          <ul className='divide-y text-sm'>
            {preview.kind === 'tables' &&
              Object.entries(preview.tables).map(([table, plan]) => (
                <li key={table} className='py-2 first:pt-0 last:pb-0'>
                  {t('settings.content.importExport.importPreviewRow', {
                    table,
                    toInsert: plan.toInsert,
                    duplicates: plan.duplicates,
                    skipped: plan.skippedRows,
                  })}
                </li>
              ))}
          </ul>
        )}
      </div>
    </section>
  )
}
