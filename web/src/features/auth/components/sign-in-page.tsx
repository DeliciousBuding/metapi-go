// metapi-go/features/auth — sign-in page.
// Centered card with brand mark + tagline + LoginForm + privacy note.
// This is the skeleton demo page proving the full stack runs (RHF + Zod +
// TanStack Query + i18n + Router + auth-session + Zustand).

import { useTranslation } from 'react-i18next'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { metapiIdentity } from '@/lib/identity-branding'

import { LoginForm } from './login-form'

type SignInPageProps = {
  redirectTo?: string
}

export function SignInPage({ redirectTo }: SignInPageProps) {
  const { t } = useTranslation()

  return (
    <div className='bg-background relative flex min-h-svh items-center justify-center p-4'>
      {/* Soft brand-tinted glow behind the card (pointer-events-none, theme-safe). */}
      <div
        aria-hidden='true'
        className='pointer-events-none fixed inset-0 overflow-hidden'
      >
        <div className='bg-primary/10 absolute -top-32 left-1/2 h-80 w-[42rem] -translate-x-1/2 rounded-full blur-3xl' />
      </div>
      <Card className='relative w-full max-w-sm shadow-sm'>
        <CardHeader className='gap-2 text-center'>
          <img
            src={metapiIdentity.logoPath}
            alt={metapiIdentity.name}
            className='mx-auto size-12'
          />
          <CardTitle className='text-2xl tracking-tight'>
            {t('auth.login.brandName')}
          </CardTitle>
          <CardDescription>{t('auth.login.brandTagline')}</CardDescription>
        </CardHeader>
        <CardContent className='grid gap-4'>
          <LoginForm redirectTo={redirectTo} />
          <p className='text-muted-foreground text-center text-xs text-pretty'>
            {t('auth.login.note')}
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
