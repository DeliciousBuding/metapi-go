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
 * The only notice currently carried by `?reason=`. Sent by the settings
 * screen after rotating the admin token (the old token stops working
 * immediately, so the operator must sign in again with the new one).
 */
const TOKEN_CHANGED_NOTICE_REASON = 'tokenChanged'

type SignInPageProps = {
  redirectTo?: string
  noticeReason?: string
}

export function SignInPage({ redirectTo, noticeReason }: SignInPageProps) {
  const { t } = useTranslation()
  const showTokenChangedNotice =
    noticeReason === TOKEN_CHANGED_NOTICE_REASON

  return (
    <div className='bg-background relative flex min-h-svh items-center justify-center px-4 pt-16 pb-4'>
      <InterfaceControls className='absolute top-3 right-3 sm:top-4 sm:right-4' />
      <Card className='relative w-full max-w-sm shadow-sm'>
        <CardHeader className='gap-2 text-center'>
          <img
            src={metapiIdentity.logoPath}
            alt={metapiIdentity.name}
            className='mx-auto size-12'
          />
          <h1 className='text-2xl font-normal tracking-tight'>
            {t('auth.login.brandName')}
          </h1>
          <CardDescription>{t('auth.login.brandTagline')}</CardDescription>
        </CardHeader>
        <CardContent className='grid gap-4'>
          {showTokenChangedNotice && (
            <div
              role='status'
              className='bg-muted/60 text-foreground flex items-start gap-2 rounded-md px-3 py-2.5 text-sm'
            >
              <Info aria-hidden='true' className='mt-0.5 size-4 shrink-0' />
              <span>{t('auth.login.tokenChangedNotice')}</span>
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
                className='text-primary underline underline-offset-2'
              >
                {t('auth.login.tokenSourceDocs')}
              </a>
            </p>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
