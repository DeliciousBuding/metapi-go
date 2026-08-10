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

import { LoginForm } from './login-form'

type SignInPageProps = {
  redirectTo?: string
}

export function SignInPage({ redirectTo }: SignInPageProps) {
  const { t } = useTranslation()

  return (
    <div className='flex min-h-svh items-center justify-center bg-background p-4'>
      <Card className='w-full max-w-sm'>
        <CardHeader className='text-center'>
          <div className='mx-auto mb-2 flex size-12 items-center justify-center rounded-xl bg-primary text-primary-foreground'>
            <span className='text-lg font-bold'>M</span>
          </div>
          <CardTitle className='text-2xl'>{t('auth.login.brandName')}</CardTitle>
          <CardDescription>{t('auth.login.brandTagline')}</CardDescription>
        </CardHeader>
        <CardContent className='grid gap-4'>
          <LoginForm redirectTo={redirectTo} />
          <p className='text-center text-xs text-muted-foreground'>
            {t('auth.login.note')}
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
