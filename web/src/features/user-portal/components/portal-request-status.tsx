import { useTranslation } from 'react-i18next'

import {
  SoftToneBadge,
  type SoftTone,
} from '@/components/common/soft-tone-badge'

import { requestStatuses } from '../request-search'

export function isKnownRequestStatus(status: string) {
  return requestStatuses.some((known) => known === status)
}

function requestStatusTone(status: string): SoftTone {
  if (status === 'completed') return 'success'
  if (status === 'client_cancelled') return 'neutral'
  if (
    status === 'rejected_no_route' ||
    status === 'rejected_policy' ||
    status === 'output_interrupted'
  ) {
    return 'warning'
  }
  if (status === 'upstream_failed' || status === 'gateway_failed') {
    return 'destructive'
  }
  return 'neutral'
}

export function RequestStatusBadge({ status }: { status: string }) {
  const { t } = useTranslation()
  return (
    <SoftToneBadge tone={requestStatusTone(status)}>
      {isKnownRequestStatus(status)
        ? t('userPortal.status.' + status)
        : t('userPortal.unknown')}
    </SoftToneBadge>
  )
}
