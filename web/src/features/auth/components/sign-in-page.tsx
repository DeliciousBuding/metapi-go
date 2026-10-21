import { Info } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { InterfaceControls } from '@/components/layout/components/interface-controls'
import { BrandLogo } from '@/components/ui/brand-logo'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
} from '@/components/ui/card'

import { LoginForm } from './login-form'

/**
 * Notices carried by `?reason=`. `tokenChanged` is sent by the settings
 * screen after rotating the admin token (the old token stops working
 * immediately, so the operator must sign in again with the new one);
 * `sessionExpired` comes from the authenticated route guard when a stored
 * token has passed its 12h TTL (otherwise the deep link would silently
 * bounce back to login with no explanation).
 */
const TOKEN_CHANGED_NOTICE_REASON = 'tokenChanged'
const SESSION_EXPIRED_NOTICE_REASON = 'sessionExpired'

const NOTICE_REASON_KEY_MAP: Record<string, string> = {
  [TOKEN_CHANGED_NOTICE_REASON]: 'auth.login.tokenChangedNotice',
  [SESSION_EXPIRED_NOTICE_REASON]: 'auth.login.sessionExpiredNotice',
}

type SignInPageProps = {
  redirectTo?: string
  noticeReason?: string
}

export function SignInPage({ redirectTo, noticeReason }: SignInPageProps) {
  const { t } = useTranslation()
  const noticeReasonKey = NOTICE_REASON_KEY_MAP[noticeReason ?? ''] ?? null

  return (
    <div className='bg-background relative flex min-h-svh items-center justify-center px-4 pt-16 pb-4'>
      {/* Keep the primary login task before the corner controls in tab order. */}
      <Card className='relative w-full max-w-sm shadow-sm'>
        <CardHeader className='gap-2 text-center'>
          <BrandLogo className='mx-auto size-12' />
          <h1 className='page-title-overview'>{t('auth.login.brandName')}</h1>
          <CardDescription>{t('auth.login.brandTagline')}</CardDescription>
        </CardHeader>
        <CardContent className='grid gap-4'>
          {noticeReasonKey && (
            <div
              role='status'
              className='bg-muted/60 text-foreground flex items-start gap-2 rounded-md px-3 py-2.5 text-sm'
            >
              <Info aria-hidden='true' className='mt-0.5 size-4 shrink-0' />
              <span>{t(noticeReasonKey)}</span>
            </div>
          )}
          <LoginForm redirectTo={redirectTo} />
        </CardContent>
      </Card>
      <InterfaceControls className='absolute top-3 right-3 sm:top-4 sm:right-4' />
    </div>
  )
}
