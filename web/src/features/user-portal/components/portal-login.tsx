import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { InterfaceControls } from '@/components/layout/components/interface-controls'
import { BrandLogo } from '@/components/ui/brand-logo'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
} from '@/components/ui/card'
import { Form } from '@/components/ui/form'
import { Notice } from '@/components/ui/notice'
import { Spinner } from '@/components/ui/spinner'

import { portalQueryRoot, userPortalApi } from '../api'
import { PortalField } from './portal-field'

const loginSchema = z.object({
  username: z.string().trim().min(1, 'userPortal.required').max(64),
  password: z.string().min(1, 'userPortal.required'),
  code: z.string(),
})
type LoginValues = z.infer<typeof loginSchema>

export function PortalLogin() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [activation, setActivation] = useState(false)
  const [failed, setFailed] = useState(false)
  const form = useForm<LoginValues>({
    resolver: zodResolver(loginSchema),
    defaultValues: { username: '', password: '', code: '' },
  })
  async function submit(values: LoginValues) {
    setFailed(false)
    if (activation) {
      const bytes = new TextEncoder().encode(values.password).length
      if (bytes < 12 || bytes > 72) {
        form.setError('password', { message: 'userPortal.passwordRule' })
        return
      }
      if (!values.code.trim()) {
        form.setError('code', { message: 'userPortal.required' })
        return
      }
    }
    try {
      if (activation)
        await userPortalApi.activate({
          username: values.username,
          password: values.password,
          code: values.code.trim(),
        })
      else
        await userPortalApi.login({
          username: values.username,
          password: values.password,
        })
      form.reset()
      await client.cancelQueries({ queryKey: portalQueryRoot })
      client.removeQueries({
        queryKey: portalQueryRoot,
        predicate: (q) => q.queryKey[1] !== 'session',
      })
      client.setQueryData(
        ['userPortal', 'session'],
        await userPortalApi.session()
      )
    } catch {
      setFailed(true)
    }
  }
  return (
    <div className='bg-background relative flex min-h-svh items-center justify-center px-4 pt-16 pb-6'>
      <Card className='w-full max-w-sm shadow-sm'>
        <CardHeader className='gap-2 text-center'>
          <BrandLogo className='mx-auto size-10' />
          <h1 className='page-title'>{t('userPortal.signIn')}</h1>
          <CardDescription>
            {t(
              activation ? 'userPortal.activationHint' : 'userPortal.signInHint'
            )}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-4'>
          {failed && (
            <Notice tone='destructive' role='alert'>
              {t('userPortal.authFailed')}
            </Notice>
          )}
          <Form {...form}>
            <form onSubmit={form.handleSubmit(submit)} className='grid gap-4'>
              <PortalField
                control={form.control}
                name='username'
                label='userPortal.username'
                autoComplete='username'
              />
              {activation && (
                <PortalField
                  control={form.control}
                  name='code'
                  label='userPortal.activationCode'
                  autoComplete='off'
                />
              )}
              <PortalField
                control={form.control}
                name='password'
                label={
                  activation ? 'userPortal.newPassword' : 'userPortal.password'
                }
                type='password'
                autoComplete={activation ? 'new-password' : 'current-password'}
              />
              {activation && (
                <p className='text-muted-foreground text-xs'>
                  {t('userPortal.passwordRule')}
                </p>
              )}
              <Button type='submit' disabled={form.formState.isSubmitting}>
                {form.formState.isSubmitting && <Spinner />}
                {t(activation ? 'userPortal.activate' : 'userPortal.signIn')}
              </Button>
              <Button
                variant='ghost'
                type='button'
                disabled={form.formState.isSubmitting}
                onClick={() => {
                  setActivation(!activation)
                  setFailed(false)
                  form.reset()
                }}
              >
                {t(
                  activation
                    ? 'userPortal.backToLogin'
                    : 'userPortal.activateAccount'
                )}
              </Button>
            </form>
          </Form>
        </CardContent>
      </Card>
      <InterfaceControls className='absolute top-3 right-3' />
    </div>
  )
}
