// metapi-go/features/channels/components — cooldown root-cause dialog (P0-3).
//
// Opens from the status badge of a cooling / breaker-open channel. Renders the
// structured reason recorded when the cooldown triggered (trigger code,
// sanitized error summary, recorded-at) plus a live remaining-time countdown,
// and reuses the existing route-scoped clear-cooldown mutation as its only
// recovery action. Rows cooled before the reason schema existed show an honest
// "reason not recorded" state instead of guessing.

import { useQueryClient } from '@tanstack/react-query'
import { Snowflake } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Spinner } from '@/components/ui/spinner'
import { useClearRouteCooldown } from '@/features/token-routes/api'
import { toBcp47 } from '@/i18n/languages'
import { formatDateTime } from '@/lib/format'

import { channelsKeys, type ChannelRow } from '../types'

type CooldownReasonDialogProps = {
  channel: ChannelRow | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

/** Known backend trigger codes — keep in sync with routing/cooldown_reason.go. */
const KNOWN_REASON_CODES = [
  'usage_limit',
  'rate_limited',
  'auth_error',
  'upstream_error',
  'client_error',
  'timeout',
  'network_error',
  'probe_failure',
  'unknown',
] as const

/** HH:MM:SS style remaining time; locale-neutral on purpose (countdown). */
function formatRemaining(ms: number): string {
  const totalSeconds = Math.max(0, Math.floor(ms / 1000))
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  const pad = (n: number) => String(n).padStart(2, '0')
  return hours > 0
    ? `${pad(hours)}:${pad(minutes)}:${pad(seconds)}`
    : `${pad(minutes)}:${pad(seconds)}`
}

function useRemainingMs(
  untilIso: string | null,
  active: boolean
): number | null {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!active || !untilIso) return
    setNow(Date.now())
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [active, untilIso])
  if (!untilIso) return null
  const target = new Date(untilIso).getTime()
  if (Number.isNaN(target)) return null
  return Math.max(0, target - now)
}

export function CooldownReasonDialog({
  channel,
  open,
  onOpenChange,
}: CooldownReasonDialogProps) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const queryClient = useQueryClient()
  const clearCooldownMutation = useClearRouteCooldown()

  const remainingMs = useRemainingMs(channel?.cooldownUntil ?? null, open)

  if (!channel) {
    return <Dialog open={open} onOpenChange={onOpenChange} />
  }

  const hasReason =
    channel.cooldownReasonCode !== null ||
    channel.cooldownReason !== null ||
    channel.cooldownReasonAt !== null

  // Localized label for known codes; unknown/legacy codes fall back to the raw
  // value so the dialog never hides what the backend actually recorded.
  const codeLabel =
    channel.cooldownReasonCode !== null &&
    (KNOWN_REASON_CODES as readonly string[]).includes(
      channel.cooldownReasonCode
    )
      ? t(`channels.reason.codes.${channel.cooldownReasonCode}`)
      : (channel.cooldownReasonCode ?? '—')

  let remainingLabel: string
  if (remainingMs === null) {
    remainingLabel = t('channels.detail.notAvailable')
  } else if (remainingMs === 0) {
    remainingLabel = t('channels.reason.remainingExpired')
  } else {
    remainingLabel = formatRemaining(remainingMs)
  }

  function handleClear() {
    if (!channel) return
    clearCooldownMutation.mutate(channel.routeId, {
      onSuccess: () => {
        void queryClient.invalidateQueries({ queryKey: channelsKeys.all })
        onOpenChange(false)
      },
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>{t('channels.reason.dialogTitle')}</DialogTitle>
          <DialogDescription>
            {t('channels.reason.dialogDescription', { name: channel.name })}
          </DialogDescription>
        </DialogHeader>

        <div className='flex flex-col gap-3 text-sm'>
          {hasReason ? (
            <>
              <div className='grid grid-cols-[auto_1fr] gap-x-3 gap-y-2'>
                <span className='text-muted-foreground'>
                  {t('channels.reason.triggerCode')}
                </span>
                <span>
                  {codeLabel}
                  {channel.cooldownReasonCode ? (
                    <code className='bg-muted/60 ml-2 rounded px-1.5 py-0.5 font-mono text-xs'>
                      {channel.cooldownReasonCode}
                    </code>
                  ) : null}
                </span>
                <span className='text-muted-foreground'>
                  {t('channels.reason.recordedAt')}
                </span>
                <span className='tabular-nums'>
                  {formatDateTime(channel.cooldownReasonAt, locale)}
                </span>
                <span className='text-muted-foreground'>
                  {t('channels.reason.remaining')}
                </span>
                <span className='tabular-nums'>{remainingLabel}</span>
              </div>

              <div>
                <div className='text-muted-foreground mb-1'>
                  {t('channels.reason.errorSummary')}
                </div>
                {channel.cooldownReason ? (
                  <code className='bg-muted/40 block max-h-40 overflow-y-auto rounded-lg border p-2 font-mono text-xs break-all whitespace-pre-wrap'>
                    {channel.cooldownReason}
                  </code>
                ) : (
                  <p className='text-muted-foreground text-xs'>
                    {t('channels.reason.noSummary')}
                  </p>
                )}
              </div>
            </>
          ) : (
            <div className='bg-muted/40 rounded-lg border p-3'>
              <p className='font-medium'>{t('channels.reason.notRecorded')}</p>
              <p className='text-muted-foreground mt-1 text-xs'>
                {t('channels.reason.notRecordedHint')}
              </p>
            </div>
          )}
        </div>

        <DialogFooter className='sm:flex-row sm:items-center'>
          <p className='text-muted-foreground max-w-[240px] text-xs sm:mr-auto'>
            {t('channels.detail.clearRouteCooldownHint')}
          </p>
          <Button
            type='button'
            variant='outline'
            onClick={handleClear}
            disabled={clearCooldownMutation.isPending}
          >
            {clearCooldownMutation.isPending ? <Spinner /> : <Snowflake />}
            {t('channels.detail.clearRouteCooldown')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
