// metapi-go/features/settings/components — load-failure state for settings
// sections. Rendered instead of the form when GET /api/settings/runtime
// fails: the user sees an explicit error instead of hardcoded defaults and
// cannot accidentally save defaults over real config.

import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

type SettingsSectionErrorProps = {
  title: string
  onRetry: () => void
}

export function SettingsSectionError({
  title,
  onRetry,
}: SettingsSectionErrorProps) {
  const { t } = useTranslation()
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        <CardDescription>{t('settings.common.loadFailed')}</CardDescription>
      </CardHeader>
      <CardContent>
        <Button type='button' variant='outline' size='sm' onClick={onRetry}>
          {t('settings.common.retry')}
        </Button>
      </CardContent>
    </Card>
  )
}
