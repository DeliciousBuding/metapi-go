// metapi-go/components/common — load-failure card for a section that fetches
// its own data. Rendered instead of the section body when the query fails, so
// the operator sees an explicit error + Retry rather than an empty table that
// reads as "no data". The settings sections are the case that made this
// shared: without it a failed GET /api/settings/runtime renders hardcoded
// defaults that can then be saved over the real configuration.
//
// The caller owns the failure copy (`messageKey`) — the same contract
// `QueryErrorBanner` uses — because what failed, and what the consequence is,
// differs per section.

import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

type SectionErrorProps = {
  title: string
  /** i18n key for the failure description (e.g. `settings.common.loadFailed`). */
  messageKey: string
  onRetry: () => void
}

export function SectionError({
  title,
  messageKey,
  onRetry,
}: SectionErrorProps) {
  const { t } = useTranslation()
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{t(messageKey)}</CardDescription>
      </CardHeader>
      <CardContent>
        <Button type='button' variant='outline' size='sm' onClick={onRetry}>
          {t('common.retry')}
        </Button>
      </CardContent>
    </Card>
  )
}
