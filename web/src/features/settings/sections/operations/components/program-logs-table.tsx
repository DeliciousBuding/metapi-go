import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import {
  eventTitleSlug,
  formatTimestamp,
  type ProgramEvent,
} from '../lib/event-normalize'
import { EventMessage } from './program-event-message'

function levelVariant(level?: string): 'default' | 'secondary' | 'destructive' {
  if (level === 'error') {
    return 'destructive'
  }
  if (level === 'warning') {
    return 'default'
  }
  return 'secondary'
}

/**
 * Localized event title. Structured rows render straight from their
 * registry titleKey; legacy rows match the persisted English title against
 * the historical slug map, and unknown titles render as-is (honest residual).
 */
function EventTitle({ event }: { event: ProgramEvent }) {
  const { t } = useTranslation()
  if (event.titleKey) {
    return t(`events.titles.${event.titleKey}`, {
      defaultValue: event.title,
    })
  }
  const slug = eventTitleSlug(event.title)
  return slug ? t(`events.titles.${slug}`) : event.title
}

export function ProgramLogsTable({
  items,
  markingRead,
  onMarkRead,
}: {
  items: ProgramEvent[]
  markingRead: boolean
  onMarkRead: (id: number) => void
}) {
  const { t } = useTranslation()
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>
            {t('settings.operations.programLogs.columns.time')}
          </TableHead>
          <TableHead>
            {t('settings.operations.programLogs.columns.level')}
          </TableHead>
          <TableHead>
            {t('settings.operations.programLogs.columns.type')}
          </TableHead>
          <TableHead>
            {t('settings.operations.programLogs.columns.title')}
          </TableHead>
          <TableHead className='text-right'>
            {t('settings.operations.programLogs.columns.actions')}
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((event) => (
          <TableRow key={event.id}>
            <TableCell className='text-muted-foreground text-xs'>
              {formatTimestamp(event.createdAt)}
            </TableCell>
            <TableCell>
              <Badge variant={levelVariant(event.level)}>
                {t(
                  `settings.operations.programLogs.level.${event.level ?? 'info'}`
                )}
              </Badge>
            </TableCell>
            <TableCell className='text-xs'>
              {t(`settings.operations.programLogs.type.${event.type ?? ''}`, {
                defaultValue: event.type ?? '—',
              })}
            </TableCell>
            <TableCell>
              <div className='flex flex-col'>
                <span
                  className={
                    event.read ? 'text-muted-foreground' : 'font-medium'
                  }
                >
                  <EventTitle event={event} />
                </span>
                <EventMessage event={event} />
              </div>
            </TableCell>
            <TableCell className='text-right'>
              {event.read ? null : (
                <Button
                  type='button'
                  variant='ghost'
                  size='sm'
                  disabled={markingRead}
                  onClick={() => onMarkRead(event.id)}
                >
                  {t('settings.operations.programLogs.markRead')}
                </Button>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}
