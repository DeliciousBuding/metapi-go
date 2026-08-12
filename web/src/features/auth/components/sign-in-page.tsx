// metapi-go/features/auth — sign-in page.
// Centered card with brand mark + tagline + LoginForm + privacy note.
// This is the skeleton demo page proving the full stack runs (RHF + Zod +
// TanStack Query + i18n + Router + auth-session + Zustand).

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

type SignInPageProps = {
  redirectTo?: string
}

export function SignInPage({ redirectTo }: SignInPageProps) {
  const { t } = useTranslation()

  return (
    <div className='relative flex min-h-svh items-center justify-center bg-background px-4 pt-16 pb-4'>
      <InterfaceControls className='absolute top-3 right-3 sm:top-4 sm:right-4' />
      <Card className='shadow-sm relative w-full max-w-sm'>
        <CardHeader className='gap-2 text-center'>
          <img
            src={metapiIdentity.logoPath}
            alt={metapiIdentity.name}
            className='mx-auto size-12'
          />
          <h1 className='text-2xl font-medium tracking-tight'>
            {t('auth.login.brandName')}
          </h1>
          <CardDescription>{t('auth.login.brandTagline')}</CardDescription>
        </CardHeader>
        <CardContent className='grid gap-4'>
          <LoginForm redirectTo={redirectTo} />
          <p className='text-pretty text-center text-xs text-muted-foreground'>
            {t('auth.login.note')}
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
