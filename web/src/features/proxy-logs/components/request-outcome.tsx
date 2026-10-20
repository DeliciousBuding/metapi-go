import { CheckCircle2, CircleSlash, ShieldX, TriangleAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import type { GatewayRequest } from '@/lib/api'
import { cn } from '@/lib/utils'

export function RequestOutcome({ request }: { request: GatewayRequest }) {
  const { t } = useTranslation()
  const status = request.terminalStatus
  let Icon = TriangleAlert
  let tone = 'text-destructive-soft-fg'
  let response =
    request.httpStatus === null
      ? t('requestLogs.noStatus')
      : `HTTP ${request.httpStatus}`
  if (request.transport === 'websocket') response = t('requestLogs.wsTurn')
  if (status === 'completed') {
    Icon = CheckCircle2
    tone = 'text-success'
  } else if (status === 'client_cancelled') {
    Icon = CircleSlash
    tone = 'text-muted-foreground'
  } else if (status === 'rejected_no_route' || status === 'rejected_policy') {
    Icon = ShieldX
    tone = 'text-warning'
  }
  return (
    <div className='flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5'>
      <span
        className={cn(
          'inline-flex items-center gap-1.5 text-sm font-medium',
          tone
        )}
      >
        <Icon className='size-3.5 shrink-0' aria-hidden='true' />
        {t(`requestLogs.status.${status}`)}
      </span>
      <span className='text-muted-foreground text-xs tabular-nums'>
        {response}
      </span>
    </div>
  )
}
