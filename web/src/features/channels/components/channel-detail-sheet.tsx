// metapi-go/features/channels/components — channel detail Sheet (side panel).
//
// Opens from the row "view details" action. Mirrors the model detail sheet
// pattern: a read-only side panel that renders fields already present on the
// list row (no extra fetch — `useChannels` returns the full projection).
// The panel surfaces the routing-health vocabulary the channels page already
// displays (status / cooldown / avg latency / models) in a denser layout so
// an operator can inspect a single channel without losing the list context.

import { Ban, CheckCircle2, Clock, TriangleAlert } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { cn } from '@/lib/utils'

import type { ChannelRow, ChannelStatus } from '../types'

type ChannelDetailSheetProps = {
  channel: ChannelRow | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

const STATUS_CONFIG: Record<
  ChannelStatus,
  {
    labelKey: string
    variant: 'success' | 'warning' | 'destructive' | 'secondary'
    dotClass: string
    Icon: typeof CheckCircle2
  }
> = {
  enabled: {
    labelKey: 'channels.status.enabled',
    variant: 'success',
    dotClass: 'bg-success',
    Icon: CheckCircle2,
  },
  cooldown: {
    labelKey: 'channels.status.cooldown',
    variant: 'warning',
    dotClass: 'bg-warning',
    Icon: Clock,
  },
  breaker_open: {
    labelKey: 'channels.status.breakerOpen',
    variant: 'destructive',
    dotClass: 'bg-destructive',
    Icon: TriangleAlert,
  },
  manually_disabled: {
    labelKey: 'channels.status.manuallyDisabled',
    variant: 'secondary',
    dotClass: 'bg-muted-foreground',
    Icon: Ban,
  },
}

function formatResponse(ms: number | null): string {
  if (ms === null || ms === undefined) return '—'
  return `${Math.round(ms)}ms`
}

function formatCooldown(until: string | null): string {
  if (!until) return '—'
  const date = new Date(until)
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString()
}

function DetailRow({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <div className='grid grid-cols-3 gap-2 py-1.5 text-sm'>
      <span className='text-muted-foreground col-span-1'>{label}</span>
      <div className='col-span-2 break-words'>{children}</div>
    </div>
  )
}

export function ChannelDetailSheet({
  channel,
  open,
  onOpenChange,
}: ChannelDetailSheetProps) {
  const { t } = useTranslation()

  if (!channel) {
    return (
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent side='right' />
      </Sheet>
    )
  }

  const status = channel.status
  const statusConfig = STATUS_CONFIG[status] ?? STATUS_CONFIG.enabled
  const StatusIcon = statusConfig.Icon
  const site = channel.site
  const siteLabel = site.name || `#${site.id}`
  const typeLabel = t(`channels.type.${channel.type}`)
  const models = channel.models || t('channels.detail.notAvailable')

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side='right' className='sm:max-w-md'>
        <SheetHeader>
          <div className='flex items-center gap-3 pr-6'>
            <div className='min-w-0 flex-1'>
              <SheetTitle className='truncate'>{channel.name}</SheetTitle>
              <SheetDescription className='truncate'>
                {t('channels.detail.description')}
              </SheetDescription>
            </div>
          </div>
        </SheetHeader>

        <ScrollArea className='flex-1'>
          <div className='flex flex-col gap-4 px-4 pb-4'>
            <section>
              <h3 className='text-sm font-medium'>
                {t('channels.detail.sectionOverview')}
              </h3>
              <div className='mt-2'>
                <DetailRow label={t('channels.detail.name')}>
                  <span className='font-medium'>{channel.name}</span>
                </DetailRow>
                <DetailRow label={t('channels.detail.site')}>
                  {siteLabel}
                </DetailRow>
                <DetailRow label={t('channels.detail.type')}>
                  <Badge variant='outline' className='capitalize'>
                    {typeLabel}
                  </Badge>
                </DetailRow>
                <DetailRow label={t('channels.detail.status')}>
                  <Badge variant={statusConfig.variant} className='gap-1.5'>
                    <span
                      aria-hidden='true'
                      className={cn(
                        'size-1.5 rounded-full',
                        statusConfig.dotClass
                      )}
                    />
                    <StatusIcon aria-hidden='true' />
                    {t(statusConfig.labelKey)}
                  </Badge>
                </DetailRow>
                <DetailRow label={t('channels.detail.enabled')}>
                  <Badge variant={channel.enabled ? 'success' : 'secondary'}>
                    {channel.enabled
                      ? t('channels.detail.enabled')
                      : t('channels.detail.disabled')}
                  </Badge>
                </DetailRow>
              </div>
            </section>

            <Separator />

            <section>
              <h3 className='text-sm font-medium'>
                {t('channels.detail.sectionHealth')}
              </h3>
              <div className='mt-2'>
                <DetailRow label={t('channels.detail.priority')}>
                  {channel.priority}
                </DetailRow>
                <DetailRow label={t('channels.detail.weight')}>
                  {channel.weight}
                </DetailRow>
                <DetailRow label={t('channels.detail.avgLatency')}>
                  {formatResponse(channel.responseMs)}
                </DetailRow>
                <DetailRow label={t('channels.detail.cooldownUntil')}>
                  {formatCooldown(channel.cooldownUntil)}
                </DetailRow>
                <DetailRow label={t('channels.detail.manualOverride')}>
                  {channel.manualOverride
                    ? t('channels.detail.manualOverrideActive')
                    : t('channels.detail.manualOverrideNone')}
                </DetailRow>
              </div>
            </section>

            <Separator />

            <section>
              <h3 className='text-sm font-medium'>
                {t('channels.detail.sectionModels')}
              </h3>
              <div className='bg-muted/40 mt-2 rounded-lg border p-2'>
                <div className='text-muted-foreground text-[11px]'>
                  {t('channels.detail.routePattern')}
                </div>
                <code className='block font-mono text-xs break-all'>
                  {models}
                </code>
              </div>
            </section>
          </div>
        </ScrollArea>
      </SheetContent>
    </Sheet>
  )
}
