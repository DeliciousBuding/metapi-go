import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

const EVENT_TYPES = ['checkin', 'balance', 'token', 'proxy'] as const

export function ProgramLogActions({
  markingAllRead,
  clearing,
  onMarkAllRead,
  onExport,
  onClear,
}: {
  markingAllRead: boolean
  clearing: boolean
  onMarkAllRead: () => void
  onExport: () => void
  onClear: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-wrap gap-2'>
      <Button
        type='button'
        variant='outline'
        size='sm'
        disabled={markingAllRead}
        onClick={onMarkAllRead}
      >
        {t('settings.operations.programLogs.markAllRead')}
      </Button>
      <Button type='button' variant='outline' size='sm' onClick={onExport}>
        {t('settings.operations.programLogs.exportCsv')}
      </Button>
      <Button
        type='button'
        variant='destructive'
        size='sm'
        disabled={clearing}
        onClick={onClear}
      >
        {t('settings.operations.programLogs.clear')}
      </Button>
    </div>
  )
}

export function ProgramLogFilters({
  typeFilter,
  setTypeFilter,
  unreadOnly,
  setUnreadOnly,
  count,
}: {
  typeFilter: string
  setTypeFilter: (value: string) => void
  unreadOnly: boolean
  setUnreadOnly: (value: boolean) => void
  count: number
}) {
  const { t } = useTranslation()
  return (
    <div className='mb-4 flex flex-wrap items-center gap-3'>
      <Select
        value={typeFilter}
        onValueChange={(value) => setTypeFilter(value ?? 'all')}
      >
        <SelectTrigger
          aria-label={t('settings.operations.programLogs.filterType')}
          className='w-40'
        >
          <SelectValue>
            {(selected) =>
              !selected || selected === 'all'
                ? t('settings.operations.programLogs.allTypes')
                : t(`settings.operations.programLogs.type.${String(selected)}`)
            }
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value='all'>
            {t('settings.operations.programLogs.allTypes')}
          </SelectItem>
          {EVENT_TYPES.map((typeId) => (
            <SelectItem key={typeId} value={typeId}>
              {t(`settings.operations.programLogs.type.${typeId}`)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <label className='flex items-center gap-2 text-sm'>
        <Checkbox
          checked={unreadOnly}
          onCheckedChange={(checked) => setUnreadOnly(checked === true)}
        />
        {t('settings.operations.programLogs.unreadOnly')}
      </label>
      <span className='text-muted-foreground text-xs'>
        {t('settings.operations.programLogs.loadedCount', {
          count,
        })}
      </span>
    </div>
  )
}
