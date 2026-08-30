// metapi-go/features/auth — sign-in page.
// Centered card with brand mark + tagline + LoginForm + token-source
// guidance + privacy note. This is the skeleton demo page proving the full
// stack runs (RHF + Zod + TanStack Query + i18n + Router + auth-session +
// Zustand).

import { Info } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { InterfaceControls } from '@/components/layout/components/interface-controls'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
} from '@/components/ui/card'
import { metapiIdentity } from '@/lib/identity-branding'

import { LoginForm } from './login-form'

/** Deployment guide referenced by the token-source hint (public repo doc). */
const DEPLOYMENT_DOC_URL =
  'https://github.com/DeliciousBuding/metapi-go/blob/master/docs/deployment.md'

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
      {/* DOM order = tab order: the card (the page's primary task) comes
          first so the keyboard cycle runs form → docs link → controls with
          no focus stop in between (F-line residual F). The controls stay
          visually pinned to the corner via absolute positioning. */}
      <Card className='relative w-full max-w-sm shadow-sm'>
        <CardHeader className='gap-2 text-center'>
          <img
            src={metapiIdentity.logoPath}
            alt=''
            className='mx-auto size-12'
          />
          <h1 className='text-2xl font-normal tracking-tight'>
            {t('auth.login.brandName')}
          </h1>
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
          <div className='text-muted-foreground rounded-md border px-3 py-2.5 text-xs leading-relaxed'>
            <p>{t('auth.login.tokenSourceHint')}</p>
            <p className='mt-1'>
              {t('auth.login.tokenSourceForgot')}{' '}
              <a
                href={DEPLOYMENT_DOC_URL}
                target='_blank'
                rel='noreferrer'
                // Inline text link with a ≥24px vertical hit target
                // (WCAG 2.5.8): py-2 + the 12px line makes the box ~32px;
                // negative py margins keep the visual line spacing neutral.
                // text-primary on this token is only ~2.7:1 in both themes
                // (measured), so swap to the foreground color — underline
                // keeps the affordance with real AA contrast.
                className='text-foreground -my-1 inline-block px-1 py-2 font-medium underline underline-offset-2'
              >
                {t('auth.login.tokenSourceDocs')}
              </a>
            </p>
          </div>
        </CardContent>
      </Card>
      <InterfaceControls className='absolute top-3 right-3 sm:top-4 sm:right-4' />
    </div>
  )
}
