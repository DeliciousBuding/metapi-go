// metapi-go/features/auth — login form (RHF + Zod + FormValidationFocus).
// Single admin-token field with password masking, autofocus, and inline
// error display. On success, navigates to the sanitized redirect target (or
// the dashboard). Errors are mapped to i18next keys and surfaced via
// setError so FormMessage can translate them.

import { zodResolver } from '@hookform/resolvers/zod'
import { useNavigate } from '@tanstack/react-router'
import { ArrowRight } from 'lucide-react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { sanitizeAuthRedirect } from '@/lib/helpers/sanitize-auth-redirect'
import { toast } from '@/lib/toast'

import { useLogin } from '../api'
import { loginFormSchema, type LoginFormValues } from '../lib/login-schema'

type LoginFormProps = {
  redirectTo?: string
}

export function LoginForm({ redirectTo }: LoginFormProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const login = useLogin()

  const form = useForm<LoginFormValues>({
    resolver: zodResolver(loginFormSchema),
    defaultValues: { token: '' },
  })

  async function onSubmit(values: LoginFormValues) {
    try {
      await login.mutateAsync({ token: values.token })
      toast.success(t('auth.login.welcomeBack'))
      const target =
        sanitizeAuthRedirect(redirectTo, window.location.origin) ?? '/'
      navigate({ href: target, replace: true })
    } catch (error) {
      const messageKey =
        (error as { messageKey?: string })?.messageKey ?? 'errors.login.failed'
      form.setError('token', { message: messageKey })
    }
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className='grid gap-4'>
        <FormField
          control={form.control}
          name='token'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('auth.login.tokenLabel')}</FormLabel>
              <FormControl>
                <Input
                  type='password'
                  placeholder={t('auth.login.tokenPlaceholder')}
                  // `current-password` marks this as an existing credential so
                  // password managers offer to fill it (never to overwrite).
                  // Evaluated against `autoComplete=off` (#1029 batch A): off
                  // was rejected — UAs may ignore it on password fields and it
                  // blocks manager autofill; a `username` value does not apply
                  // because this token-only login flow has no username field.
                  autoComplete='current-password'
                  autoFocus
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <Button
          type='submit'
          size='lg'
          disabled={login.isPending}
          className='w-full'
        >
          {login.isPending ? (
            <>
              <Spinner />
              {t('auth.login.submitting')}
            </>
          ) : (
            <>
              {t('auth.login.submit')}
              <ArrowRight data-icon='inline-end' className='size-4' />
            </>
          )}
        </Button>
      </form>
    </Form>
  )
}
