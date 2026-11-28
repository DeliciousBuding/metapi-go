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
import type { Announcement } from '@/lib/api'

export function AnnouncementList({
  items,
  onEdit,
  onDelete,
}: {
  items: Announcement[]
  onEdit: (announcement: Announcement) => void
  onDelete: (announcement: Announcement) => void
}) {
  const { t } = useTranslation()
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>
            {t('settings.content.announcements.columns.severity')}
          </TableHead>
          <TableHead>
            {t('settings.content.announcements.columns.title')}
          </TableHead>
          <TableHead>
            {t('settings.content.announcements.columns.enabled')}
          </TableHead>
          <TableHead className='text-right'>
            {t('settings.content.announcements.columns.actions')}
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((announcement) => (
          <TableRow key={announcement.id}>
            <TableCell>
              <Badge variant={severityVariant(announcement.severity)}>
                {t(
                  `settings.content.announcements.severity.${announcement.severity}`
                )}
              </Badge>
            </TableCell>
            <TableCell>
              <div className='flex flex-col'>
                <span className='font-medium'>{announcement.title}</span>
                <span className='text-muted-foreground line-clamp-2 text-xs'>
                  {announcement.message}
                </span>
              </div>
            </TableCell>
            <TableCell>
              <Badge variant={announcement.enabled ? 'default' : 'secondary'}>
                {announcement.enabled
                  ? t('settings.common.enabled')
                  : t('settings.common.disabled')}
              </Badge>
            </TableCell>
            <TableCell className='text-right'>
              <div className='flex justify-end gap-1'>
                <Button
                  type='button'
                  variant='ghost'
                  size='sm'
                  onClick={() => onEdit(announcement)}
                >
                  {t('settings.common.edit')}
                </Button>
                <Button
                  type='button'
                  variant='ghost'
                  size='sm'
                  onClick={() => onDelete(announcement)}
                >
                  {t('settings.common.delete')}
                </Button>
              </div>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

function severityVariant(
  severity: Announcement['severity']
): 'default' | 'secondary' | 'destructive' {
  if (severity === 'critical') {
    return 'destructive'
  }
  if (severity === 'warning') {
    return 'default'
  }
  return 'secondary'
}
