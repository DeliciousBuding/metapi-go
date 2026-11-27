import {
  GripVertical as GripVerticalIcon,
  Pencil as PencilIcon,
  RefreshCw as RefreshCwIcon,
  Trash2 as Trash2Icon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { toBcp47 } from '@/i18n/languages'
import type { CatalogSource } from '@/lib/api'
import { formatDateTime } from '@/lib/format'
import { cn } from '@/lib/utils'

import { useCatalogSourceOrder } from './use-catalog-source-order'
import type { useCatalogSources } from './use-catalog-sources'

function LastSyncCell({ source }: { source: CatalogSource }) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  if (!source.lastAttemptAt) {
    return (
      <span className='text-muted-foreground text-sm'>
        {t('settings.proxyModels.catalogSources.neverSynced')}
      </span>
    )
  }
  const at = formatDateTime(source.lastAttemptAt, locale)
  const count = t('settings.proxyModels.catalogSources.modelCount', {
    count: source.lastCount,
  })
  if (source.lastError) {
    return (
      <span className='flex flex-col'>
        <span className='text-sm'>{at}</span>
        <span
          className='text-destructive-soft-fg max-w-64 truncate text-xs max-sm:max-w-none max-sm:[overflow-wrap:anywhere] max-sm:whitespace-normal'
          title={source.lastError}
        >
          {source.lastError}
        </span>
      </span>
    )
  }
  return (
    <span className='flex flex-col'>
      <span className='text-sm'>{at}</span>
      <span className='text-muted-foreground text-xs'>{count}</span>
    </span>
  )
}

export function CatalogSourceTable({
  sources,
  syncingId,
  runtimeEnabled,
  actions,
  onSync,
  onEdit,
}: {
  sources: CatalogSource[]
  syncingId: number | 'all' | null
  runtimeEnabled: boolean
  actions: ReturnType<typeof useCatalogSources>['sourceActions']
  onSync: (id: number) => void
  onEdit: (source: CatalogSource) => void
}) {
  const { t } = useTranslation()
  const busy = syncingId !== null
  const { rowRefs, dragIndex, overIndex, handleProps } = useCatalogSourceOrder(
    sources,
    busy || actions.reordering,
    actions.reorder
  )
  if (sources.length === 0)
    return (
      <p className='text-muted-foreground py-4 text-center text-sm'>
        {t('settings.proxyModels.catalogSources.empty')}
      </p>
    )
  return (
    <>
      <p id='catalog-source-reorder-help' className='sr-only'>
        {t('settings.proxyModels.catalogSources.reorderInstructions')}
      </p>
      <Table className='max-sm:block'>
        <TableHeader className='max-sm:sr-only'>
          <TableRow>
            {/* Drag-handle column (reorder). The
                        header cell carries an sr-only label so table scans do
                        not read an empty th (axe empty-table-header). */}
            <TableHead className='w-7'>
              <span className='sr-only'>
                {t('settings.proxyModels.catalogSources.columns.reorderColumn')}
              </span>
            </TableHead>
            <TableHead>
              {t('settings.proxyModels.catalogSources.columns.source')}
            </TableHead>
            <TableHead>
              {t('settings.proxyModels.catalogSources.columns.type')}
            </TableHead>
            <TableHead>
              {t('settings.proxyModels.catalogSources.columns.url')}
            </TableHead>
            <TableHead>
              {t('settings.proxyModels.catalogSources.columns.enabled')}
            </TableHead>
            <TableHead>
              {t('settings.proxyModels.catalogSources.columns.lastSync')}
            </TableHead>
            <TableHead className='text-right'>
              {t('settings.proxyModels.catalogSources.columns.actions')}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody className='max-sm:block max-sm:[&>tr]:h-auto'>
          {sources.map((source, index) => (
            <TableRow
              key={source.id}
              ref={(row) => {
                rowRefs.current[index] = row
              }}
              className={cn(
                'max-sm:grid max-sm:grid-cols-[auto_minmax(0,1fr)_auto] max-sm:items-center max-sm:gap-x-2 max-sm:gap-y-3 max-sm:p-3',
                dragIndex !== null && overIndex === index && 'bg-accent/60',
                dragIndex === index && 'opacity-40'
              )}
            >
              <TableCell className='w-7 px-0 max-sm:col-start-1 max-sm:row-start-1 max-sm:p-0'>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon-sm'
                  className={cn(
                    'cursor-grab touch-none select-none',
                    dragIndex === index && 'cursor-grabbing'
                  )}
                  {...handleProps(index)}
                  aria-label={t(
                    'settings.proxyModels.catalogSources.reorderRow',
                    { name: source.name }
                  )}
                  aria-keyshortcuts='Enter Space ArrowUp ArrowDown Escape'
                  aria-describedby='catalog-source-reorder-help'
                  title={t('settings.proxyModels.catalogSources.dragToReorder')}
                >
                  <GripVerticalIcon className='size-3.5' />
                </Button>
              </TableCell>
              <TableCell className='text-sm font-medium max-sm:col-start-2 max-sm:row-start-1 max-sm:min-w-0 max-sm:p-0 max-sm:[overflow-wrap:anywhere] max-sm:whitespace-normal'>
                {source.name}
              </TableCell>
              <TableCell className='max-sm:col-start-3 max-sm:row-start-1 max-sm:p-0'>
                <Badge
                  variant={source.type === 'official' ? 'secondary' : 'outline'}
                >
                  {source.type === 'official'
                    ? t('settings.proxyModels.catalogSources.typeOfficial')
                    : t('settings.proxyModels.catalogSources.typeCustom')}
                </Badge>
              </TableCell>
              <TableCell
                className='text-muted-foreground max-w-64 truncate font-mono text-xs max-sm:col-span-3 max-sm:row-start-2 max-sm:max-w-none max-sm:p-0 max-sm:[overflow-wrap:anywhere] max-sm:whitespace-normal'
                title={source.url}
              >
                {source.url}
              </TableCell>
              <TableCell className='max-sm:col-span-3 max-sm:row-start-3 max-sm:flex max-sm:items-center max-sm:justify-between max-sm:p-0'>
                <span
                  aria-hidden='true'
                  className='text-muted-foreground hidden text-xs max-sm:inline'
                >
                  {t('settings.proxyModels.catalogSources.columns.enabled')}
                </span>
                <Switch
                  checked={source.enabled}
                  onCheckedChange={() => actions.toggleEnabled(source)}
                  disabled={actions.toggling}
                  aria-label={t(
                    'settings.proxyModels.catalogSources.toggleEnabled',
                    {
                      name: source.name,
                    }
                  )}
                />
              </TableCell>
              <TableCell className='max-sm:col-span-3 max-sm:row-start-4 max-sm:min-w-0 max-sm:p-0 max-sm:whitespace-normal'>
                <span
                  aria-hidden='true'
                  className='text-muted-foreground hidden text-xs max-sm:block max-sm:pb-1'
                >
                  {t('settings.proxyModels.catalogSources.columns.lastSync')}
                </span>
                <LastSyncCell source={source} />
              </TableCell>
              <TableCell className='max-sm:col-span-3 max-sm:row-start-5 max-sm:p-0'>
                <div className='flex justify-end gap-1 max-sm:flex-wrap max-sm:border-t max-sm:pt-2'>
                  <Button
                    variant='ghost'
                    size='icon-sm'
                    className='max-sm:w-auto max-sm:gap-1.5 max-sm:px-2'
                    disabled={busy || !runtimeEnabled}
                    onClick={() => onSync(source.id)}
                    aria-label={t(
                      'settings.proxyModels.catalogSources.syncNow'
                    )}
                  >
                    {syncingId === source.id ? (
                      <Spinner className='size-3.5' />
                    ) : (
                      <RefreshCwIcon className='size-3.5' />
                    )}
                    <span className='hidden max-sm:inline'>
                      {t('settings.proxyModels.catalogSources.syncNow')}
                    </span>
                  </Button>
                  <Button
                    variant='ghost'
                    size='icon-sm'
                    className='max-sm:w-auto max-sm:gap-1.5 max-sm:px-2'
                    onClick={() => onEdit(source)}
                    aria-label={t('settings.proxyModels.catalogSources.edit')}
                  >
                    <PencilIcon className='size-3.5' />
                    <span className='hidden max-sm:inline'>
                      {t('settings.proxyModels.catalogSources.edit')}
                    </span>
                  </Button>
                  <Button
                    variant='ghost'
                    size='icon-sm'
                    className='max-sm:w-auto max-sm:gap-1.5 max-sm:px-2'
                    onClick={() => actions.deleteSource(source)}
                    aria-label={t('settings.proxyModels.catalogSources.delete')}
                  >
                    <Trash2Icon className='size-3.5' />
                    <span className='hidden max-sm:inline'>
                      {t('settings.proxyModels.catalogSources.delete')}
                    </span>
                  </Button>
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </>
  )
}
