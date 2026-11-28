import { useTranslation } from 'react-i18next'

import {
  SoftToneBadge,
  type SoftTone,
} from '@/components/common/soft-tone-badge'

import type { OAuthClientStatus } from '../types'

const STATUS_LABEL_KEY: Record<OAuthClientStatus, string> = {
  healthy: 'oauth.status.healthy',
  abnormal: 'oauth.status.abnormal',
}

const STATUS_SOFT_TONE: Record<OAuthClientStatus, SoftTone> = {
  healthy: 'success',
  abnormal: 'destructive',
}

/**
 * Dual-channel health badge for one connection. Exported so the detail sheet
 * renders the exact same OKLCH soft-tone silhouette as the list column (a
 * private copy would let the two drift).
 */
export function OAuthStatusBadge({
  status,
}: {
  status: OAuthClientStatus | undefined
}) {
  const { t } = useTranslation()
  const resolved: OAuthClientStatus =
    status === 'abnormal' ? 'abnormal' : 'healthy'
  return (
    <SoftToneBadge tone={STATUS_SOFT_TONE[resolved]}>
      {t(STATUS_LABEL_KEY[resolved])}
    </SoftToneBadge>
  )
}
