import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import {
  SoftToneBadge,
  type SoftTone,
} from '@/components/common/soft-tone-badge'
import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
import { Form } from '@/components/ui/form'
import { Notice } from '@/components/ui/notice'
import {
  gatewayUsersApi,
  type GatewayUser,
  type UserBudget,
} from '@/lib/api/gateway-users'

import { useAdminWrite, userAdminKeys } from './api'
import { BudgetHolds } from './budget-holds'
import {
  amountInput,
  BooleanField,
  TextField,
  type EditorState,
} from './editor-fields'
import { LoadError, Loading, WriteError } from './feedback'
import { availableQuotaPreview } from './quota-preview'

export function BudgetDialog({
  user,
  close,
}: {
  user: GatewayUser
  close: () => void
}) {
  const { t } = useTranslation()
  const [budgetState, setBudgetState] = useState<EditorState>({
    dirty: false,
    pending: false,
  })
  const [holdsState, setHoldsState] = useState<EditorState>({
    dirty: false,
    pending: false,
  })
  const { handleOpenChange, guard } = useDirtyDialogClose({
    enabled: budgetState.dirty || holdsState.dirty,
    onOpenChange: (open) => {
      if (!open) close()
    },
  })
  const query = useQuery({
    queryKey: userAdminKeys.budget(user.id),
    queryFn: () => gatewayUsersApi.budget(user.id),
  })
  return (
    <>
      <Dialog
        open
        onOpenChange={(open) => {
          if (!budgetState.pending && !holdsState.pending)
            handleOpenChange(open)
        }}
      >
        <DialogContent className='max-h-[90svh] overflow-y-auto sm:max-w-lg'>
          <DialogHeader>
            <DialogTitle>
              {t('userAdmin.budgetTitle', {
                user: user.displayName || user.username,
              })}
            </DialogTitle>
            <DialogDescription>
              {t('userAdmin.budgetBoundary')}
            </DialogDescription>
          </DialogHeader>
          {query.isPending && <Loading />}
          {query.isError && (
            <LoadError
              error={query.error}
              retry={() => void query.refetch()}
              pending={query.isFetching}
            />
          )}
          {query.isSuccess && (
            <>
              <BudgetEditor
                userId={user.id}
                budget={query.data}
                onStateChange={setBudgetState}
              />
              <BudgetHolds userId={user.id} onStateChange={setHoldsState} />
            </>
          )}
        </DialogContent>
      </Dialog>
      {guard}
    </>
  )
}

export function BudgetEditor({
  userId,
  budget,
  onStateChange,
}: {
  userId: number
  budget: UserBudget
  onStateChange?: (state: EditorState) => void
}) {
  const { t } = useTranslation()
  const schema = z.object({
    limit: amountInput(t('userAdmin.amountHint')),
    unlimited: z.boolean(),
  })
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { limit: String(budget.limit), unlimited: budget.unlimited },
    values: { limit: String(budget.limit), unlimited: budget.unlimited },
    resetOptions: { keepDirtyValues: true },
  })
  const [saved, setSaved] = useState(false)
  const mutation = useAdminWrite(
    (value: z.infer<typeof schema>, token) =>
      gatewayUsersApi.setBudget(
        userId,
        { limit: Number(value.limit), unlimited: value.unlimited },
        token
      ),
    () => {
      setSaved(true)
      form.reset(form.getValues())
    }
  )
  const dirty = form.formState.isDirty
  const previewLimit = form.watch('limit')
  const previewUnlimited = form.watch('unlimited')
  const remaining = availableQuotaPreview(
    previewLimit,
    budget.used,
    budget.reserved
  )
  let quotaHealthTone: SoftTone = 'success'
  let quotaHealthLabelKey = 'userAdmin.readiness.state.passed'
  if (budget.frozen) {
    quotaHealthTone = 'destructive'
    quotaHealthLabelKey = 'userAdmin.readiness.state.action'
  } else if (
    !previewUnlimited &&
    (remaining === null || Number(remaining) <= 0)
  ) {
    quotaHealthTone = 'warning'
    quotaHealthLabelKey = 'userAdmin.readiness.state.action'
  }
  useEffect(() => {
    onStateChange?.({ dirty, pending: mutation.isPending })
  }, [dirty, mutation.isPending, onStateChange])
  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit((value) => {
          setSaved(false)
          mutation.submit(value)
        })}
        className='space-y-4'
      >
        <dl className='bg-muted/50 grid grid-cols-2 gap-3 rounded-lg border p-3 text-sm'>
          <div className='min-w-0'>
            <dt className='text-muted-foreground text-xs'>
              {t('userAdmin.used')}
            </dt>
            <dd className='mt-0.5 font-medium wrap-anywhere tabular-nums'>
              {budget.used}
            </dd>
          </div>
          <div className='min-w-0'>
            <dt className='text-muted-foreground text-xs'>
              {t('userAdmin.reserved')}
            </dt>
            <dd className='mt-0.5 font-medium wrap-anywhere tabular-nums'>
              {budget.reserved}
            </dd>
          </div>
        </dl>
        <p className='text-muted-foreground text-xs'>
          {t('userAdmin.tokenUnit')}
        </p>
        {budget.frozen && (
          <Notice tone='warning'>
            {t('userAdmin.frozen', { count: budget.unknownRequests })}
          </Notice>
        )}
        <TextField
          control={form.control}
          name='limit'
          label={t('userAdmin.limit')}
          type='number'
        />
        <BooleanField
          control={form.control}
          name='unlimited'
          label={t('userAdmin.unlimited')}
        />
        <p className='text-muted-foreground text-xs'>
          {t('userAdmin.zeroHint')}
        </p>
        <section
          className='bg-muted/50 space-y-2 rounded-lg border p-3'
          aria-live='polite'
        >
          <div className='flex flex-wrap items-center justify-between gap-2'>
            <h3 className='text-sm font-medium'>
              {t('userAdmin.budgetPreview.title')}
            </h3>
            <SoftToneBadge tone={quotaHealthTone}>
              {t(quotaHealthLabelKey)}
            </SoftToneBadge>
          </div>
          <p className='text-sm font-medium wrap-anywhere tabular-nums'>
            {t(
              previewUnlimited
                ? 'userAdmin.budgetPreview.unlimited'
                : 'userAdmin.budgetPreview.remaining',
              { amount: remaining ?? t('userAdmin.budgetPreview.invalid') }
            )}
          </p>
          {!previewUnlimited && remaining !== null && (
            <p className='text-muted-foreground text-xs wrap-anywhere tabular-nums'>
              {t('userAdmin.budgetPreview.formula', {
                limit: previewLimit,
                used: budget.used,
                reserved: budget.reserved,
                amount: remaining,
              })}
            </p>
          )}
          <p className='text-muted-foreground text-xs'>
            {t('userAdmin.budgetPreview.boundary')}
          </p>
          {budget.frozen && (
            <p className='text-destructive-soft-fg text-xs'>
              {t('userAdmin.budgetPreview.frozen')}
            </p>
          )}
        </section>
        <WriteError error={mutation.error} />
        <div className='flex items-center justify-end gap-3'>
          {saved && (
            <span role='status' className='text-muted-foreground text-sm'>
              {t('userAdmin.saved')}
            </span>
          )}
          <Button type='submit' disabled={mutation.isPending}>
            {t(mutation.isPending ? 'userAdmin.saving' : 'userAdmin.save')}
          </Button>
        </div>
      </form>
    </Form>
  )
}
