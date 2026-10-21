import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Form } from '@/components/ui/form'
import { Notice } from '@/components/ui/notice'
import { Spinner } from '@/components/ui/spinner'

import { portalQueryRoot, userPortalApi, type UserSession } from '../api'
import { PortalField } from './portal-field'

const passwordSchema = z.object({
  currentPassword: z.string().min(1, 'userPortal.required'),
  newPassword: z.string().refine((value) => {
    const length = new TextEncoder().encode(value).length
    return length >= 12 && length <= 72
  }, 'userPortal.passwordRule'),
})

export function PortalAccount({ session }: { session: UserSession }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [failed, setFailed] = useState(false)
  const form = useForm<z.infer<typeof passwordSchema>>({
    resolver: zodResolver(passwordSchema),
    defaultValues: { currentPassword: '', newPassword: '' },
  })
  return (
    <div className='max-w-xl space-y-6'>
      <dl className='grid grid-cols-2 gap-4 text-sm'>
        <div>
          <dt className='text-muted-foreground'>{t('userPortal.username')}</dt>
          <dd className='mt-1 font-medium wrap-anywhere'>
            {session.user.username}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground'>
            {t('userPortal.currentGroup')}
          </dt>
          <dd className='mt-1 font-medium wrap-anywhere'>
            {session.group.name}
          </dd>
        </div>
      </dl>
      <Card>
        <CardContent className='space-y-4'>
          <h2 className='text-base font-semibold'>
            {t('userPortal.changePassword')}
          </h2>
          <p className='text-muted-foreground text-sm'>
            {t('userPortal.passwordChangeHint')}
          </p>
          {failed && (
            <Notice tone='destructive' role='alert'>
              {t('userPortal.actionFailed')}
            </Notice>
          )}
          <Form {...form}>
            <form
              className='grid gap-4'
              onSubmit={form.handleSubmit(async (values) => {
                setFailed(false)
                try {
                  await userPortalApi.password(values)
                  form.reset()
                  client.removeQueries({
                    queryKey: portalQueryRoot,
                    predicate: (q) => q.queryKey[1] !== 'session',
                  })
                  client.setQueryData(['userPortal', 'session'], null)
                } catch {
                  setFailed(true)
                }
              })}
            >
              <PortalField
                control={form.control}
                name='currentPassword'
                label='userPortal.currentPassword'
                type='password'
                autoComplete='current-password'
              />
              <PortalField
                control={form.control}
                name='newPassword'
                label='userPortal.newPassword'
                type='password'
                autoComplete='new-password'
              />
              <Button type='submit' disabled={form.formState.isSubmitting}>
                {form.formState.isSubmitting && <Spinner />}
                {t('userPortal.changePassword')}
              </Button>
            </form>
          </Form>
        </CardContent>
      </Card>
    </div>
  )
}
