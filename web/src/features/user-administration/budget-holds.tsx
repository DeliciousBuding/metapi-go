import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import {
  Form,
  FormField,
  FormItem,
  FormLabel,
  FormControl,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Notice } from '@/components/ui/notice'
import { gatewayUsersApi } from '@/lib/api/gateway-users'

import { useAdminWrite, userAdminKeys } from './api'
import type { EditorState } from './editors'
import { LoadError, Loading, WriteError } from './feedback'

export function BudgetHolds({
  userId,
  onStateChange,
}: {
  userId: number
  onStateChange?: (state: EditorState) => void
}) {
  const { t } = useTranslation()
  const [offset, setOffset] = useState(0)
  const query = useQuery({
    queryKey: [...userAdminKeys.budget(userId), 'holds', offset],
    queryFn: () => gatewayUsersApi.holds(userId, offset),
  })
  const [selected, setSelected] = useState<string | null>(null)
  const schema = z.object({
    totalTokens: z
      .string()
      .regex(/^\d+$/, t('userAdmin.tokensHint'))
      .refine(
        (value) => Number.isSafeInteger(Number(value)),
        t('userAdmin.tokensHint')
      ),
    verified: z.string().regex(/^VERIFIED$/, t('userAdmin.verifyHint')),
  })
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { totalTokens: '', verified: '' },
  })
  const mutation = useAdminWrite(
    (value: { requestId: string; totalTokens: number }, token) =>
      gatewayUsersApi.reconcile(
        userId,
        { ...value, confirmTerminated: true },
        token
      ),
    () => {
      setSelected(null)
      form.reset()
    }
  )
  const dirty = form.formState.isDirty
  useEffect(() => {
    onStateChange?.({ dirty, pending: mutation.isPending })
  }, [dirty, mutation.isPending, onStateChange])
  return (
    <section className='space-y-3 border-t pt-4'>
      <h2 className='text-base font-semibold'>{t('userAdmin.holdsTitle')}</h2>
      <p className='text-muted-foreground text-xs'>
        {t('userAdmin.holdsHint')}
      </p>
      {query.isPending && <Loading />}
      <LoadError
        error={query.error}
        retry={() => void query.refetch()}
        pending={query.isFetching}
      />
      {query.isSuccess && !query.isError && (
        <>
          <div className='max-h-52 divide-y overflow-y-auto rounded-lg border'>
            {query.data.items.length === 0 ? (
              <p className='text-muted-foreground p-3 text-sm'>
                {t('userAdmin.noHolds')}
              </p>
            ) : (
              query.data.items.map((item) => (
                <div key={item.requestId} className='space-y-2 p-3'>
                  <p className='font-mono text-xs wrap-anywhere'>
                    {item.requestId}
                  </p>
                  <p className='text-muted-foreground text-xs'>
                    {t(
                      item.state === 'pending'
                        ? 'userAdmin.holdPending'
                        : 'userAdmin.holdUnknown'
                    )}{' '}
                    · {t('userAdmin.reserved')}: {item.reserved} · ×
                    {item.multiplier}
                  </p>
                  <Button
                    size='sm'
                    variant='outline'
                    disabled={mutation.isPending}
                    onClick={() => {
                      setSelected(item.requestId)
                      form.reset()
                    }}
                  >
                    {t('userAdmin.reconcile')}
                  </Button>
                </div>
              ))
            )}
          </div>
          <div className='flex items-center gap-2'>
            <Button
              size='sm'
              variant='ghost'
              disabled={offset === 0 || query.isFetching || mutation.isPending}
              onClick={() => {
                setOffset(Math.max(0, offset - query.data.limit))
                setSelected(null)
              }}
            >
              {t('userAdmin.previous')}
            </Button>
            <Button
              size='sm'
              variant='ghost'
              disabled={
                query.data.items.length < query.data.limit ||
                query.isFetching ||
                mutation.isPending
              }
              onClick={() => {
                setOffset(offset + query.data.limit)
                setSelected(null)
              }}
            >
              {t('userAdmin.next')}
            </Button>
            <Button
              size='sm'
              variant='ghost'
              disabled={query.isFetching}
              onClick={() => void query.refetch()}
            >
              {t('userAdmin.refresh')}
            </Button>
          </div>
        </>
      )}
      {selected && (
        <Form {...form}>
          <form
            className='space-y-3 rounded-lg border p-3'
            onSubmit={form.handleSubmit((value) =>
              mutation.submit({
                requestId: selected,
                totalTokens: Number(value.totalTokens),
              })
            )}
          >
            <Notice tone='warning'>{t('userAdmin.reconcileWarning')}</Notice>
            <p className='font-mono text-xs wrap-anywhere'>{selected}</p>
            <FormField
              control={form.control}
              name='totalTokens'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('userAdmin.verifiedTokens')}</FormLabel>
                  <FormControl>
                    <Input {...field} inputMode='numeric' />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='verified'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('userAdmin.verifyLabel')}</FormLabel>
                  <FormControl>
                    <Input {...field} autoComplete='off' />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <WriteError error={mutation.error} />
            <Button
              size='sm'
              type='submit'
              disabled={mutation.isPending || query.isFetching || query.isError}
            >
              {t(
                mutation.isPending
                  ? 'userAdmin.saving'
                  : 'userAdmin.confirmReconcile'
              )}
            </Button>
          </form>
        </Form>
      )}
    </section>
  )
}
