import { zodResolver } from '@hookform/resolvers/zod'
import { useQuery } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import {
  useForm,
  type Control,
  type FieldPath,
  type FieldValues,
} from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { useReauth } from '@/components/common/reauth-provider'
import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
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
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { toBcp47 } from '@/i18n/languages'
import {
  gatewayUsersApi,
  type GatewayUser,
  type GatewayUserGroup,
  type UserBudget,
  type GroupPricing,
} from '@/lib/api/gateway-users'
import { copyText } from '@/lib/clipboard'
import { isReauthRequired } from '@/lib/http-client'
import { useAuthStore } from '@/stores/auth-store'

import { useAdminWrite, userAdminKeys } from './api'
import { BudgetHolds } from './budget-holds'
import { LoadError, Loading, WriteError } from './feedback'
import { availableQuotaPreview, multiplierPreview } from './quota-preview'

export type EditorState = { dirty: boolean; pending: boolean }

function TextField<T extends FieldValues>({
  control,
  name,
  label,
  type = 'text',
  disabled = false,
}: {
  control: Control<T>
  name: FieldPath<T>
  label: string
  type?: string
  disabled?: boolean
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <Input
              {...field}
              value={String(field.value ?? '')}
              type={type}
              step={type === 'number' ? '0.000001' : undefined}
              disabled={disabled}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

function BooleanField<T extends FieldValues>({
  control,
  name,
  label,
}: {
  control: Control<T>
  name: FieldPath<T>
  label: string
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem className='flex items-center justify-between gap-4'>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <Switch
              name={field.name}
              checked={Boolean(field.value)}
              onCheckedChange={field.onChange}
              onBlur={field.onBlur}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

export function IdentityEditor({
  kind,
  user,
  group,
  groups,
  close,
}: {
  kind: 'user' | 'group'
  user?: GatewayUser
  group?: GatewayUserGroup
  groups: GatewayUserGroup[]
  close: () => void
}) {
  const { t } = useTranslation()
  const editing = Boolean(user || group)
  const titleKeys = {
    user: { create: 'userAdmin.createUser', edit: 'userAdmin.editUser' },
    group: { create: 'userAdmin.createGroup', edit: 'userAdmin.editGroup' },
  }
  const schema = z.object({
    identifier: editing
      ? z.string()
      : z
          .string()
          .trim()
          .regex(/^[a-z0-9_-]{1,64}$/, t('userAdmin.identifierHint')),
    name: z
      .string()
      .trim()
      .max(100, t('userAdmin.nameHint'))
      .refine(
        (value) => kind === 'user' || value.length > 0,
        t('userAdmin.nameHint')
      ),
    groupId: z.number().int().nonnegative(),
    enabled: z.boolean(),
  })
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: {
      identifier: user?.username ?? group?.slug ?? '',
      name: user?.displayName ?? group?.name ?? '',
      groupId: user?.groupId ?? 0,
      enabled: user?.enabled ?? group?.enabled ?? true,
    },
  })
  const mutation = useAdminWrite(
    async (value: z.infer<typeof schema>, token) => {
      if (kind === 'user') {
        const body = {
          displayName: value.name,
          groupId: value.groupId,
          enabled: value.enabled,
        }
        return user
          ? gatewayUsersApi.updateUser(user.id, body, token)
          : gatewayUsersApi.createUser(
              { ...body, username: value.identifier },
              token
            )
      }
      const body = { name: value.name, enabled: value.enabled }
      return group
        ? gatewayUsersApi.updateGroup(group.id, body, token)
        : gatewayUsersApi.createGroup(
            { ...body, slug: value.identifier },
            token
          )
    },
    () => {
      form.reset()
      close()
    }
  )
  const { handleOpenChange, guard } = useDirtyDialogClose({
    enabled: form.formState.isDirty,
    onOpenChange: (open) => {
      if (!open) close()
    },
  })
  return (
    <>
      <Dialog
        open
        onOpenChange={(open) => {
          if (!mutation.isPending) handleOpenChange(open)
        }}
      >
        <DialogContent className='sm:max-w-lg'>
          <DialogHeader>
            <DialogTitle>
              {t(titleKeys[kind][editing ? 'edit' : 'create'])}
            </DialogTitle>
            <DialogDescription>
              {t(
                kind === 'user'
                  ? 'userAdmin.userBoundary'
                  : 'userAdmin.groupBoundary'
              )}
            </DialogDescription>
          </DialogHeader>
          <Form {...form}>
            <form
              onSubmit={form.handleSubmit((value) => mutation.submit(value))}
              className='space-y-4'
            >
              <TextField
                control={form.control}
                name='identifier'
                label={t(
                  kind === 'user' ? 'userAdmin.username' : 'userAdmin.slug'
                )}
                disabled={editing}
              />
              {!editing && (
                <p className='text-muted-foreground text-xs'>
                  {t('userAdmin.identifierHint')}
                </p>
              )}
              <TextField
                control={form.control}
                name='name'
                label={t(
                  kind === 'user'
                    ? 'userAdmin.displayName'
                    : 'userAdmin.groupName'
                )}
              />
              {kind === 'user' && (
                <FormField
                  control={form.control}
                  name='groupId'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('userAdmin.group')}</FormLabel>
                      <Select
                        value={String(field.value)}
                        onValueChange={(value) => field.onChange(Number(value))}
                      >
                        <FormControl>
                          <SelectTrigger className='w-full'>
                            <SelectValue>
                              {field.value === 0
                                ? t('userAdmin.defaultGroup')
                                : (groups.find(
                                    (item) => item.id === field.value
                                  )?.name ?? `#${field.value}`)}
                            </SelectValue>
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent>
                          {!editing && (
                            <SelectItem value='0'>
                              {t('userAdmin.defaultGroup')}
                            </SelectItem>
                          )}
                          {groups.map((item) => (
                            <SelectItem key={item.id} value={String(item.id)}>
                              {item.name}
                              {!item.enabled
                                ? ` · ${t('userAdmin.disabled')}`
                                : ''}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
              {kind === 'user' && !editing && (
                <Notice tone='warning'>{t('userAdmin.defaultWarning')}</Notice>
              )}
              <BooleanField
                control={form.control}
                name='enabled'
                label={t('userAdmin.enabled')}
              />
              <WriteError error={mutation.error} />
              <div className='flex justify-end gap-2'>
                <Button
                  variant='outline'
                  type='button'
                  onClick={() => handleOpenChange(false)}
                  disabled={mutation.isPending}
                >
                  {t('userAdmin.cancel')}
                </Button>
                <Button type='submit' disabled={mutation.isPending}>
                  {t(
                    mutation.isPending ? 'userAdmin.saving' : 'userAdmin.save'
                  )}
                </Button>
              </div>
            </form>
          </Form>
        </DialogContent>
      </Dialog>
      {guard}
    </>
  )
}

const decimalPattern = /^(?:0|[1-9]\d*)(?:\.\d{1,6})?$/
const amountInput = (message: string, positive = false) =>
  z
    .string()
    .trim()
    .regex(decimalPattern, message)
    .refine(
      (value) =>
        Number(value) <= 1e9 &&
        (positive ? Number(value) > 0 : Number(value) >= 0),
      message
    )

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
        <DialogContent className='sm:max-w-lg'>
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
        <dl className='bg-muted grid grid-cols-2 gap-4 rounded-lg p-3 text-sm'>
          <div>
            <dt className='text-muted-foreground'>{t('userAdmin.used')}</dt>
            <dd className='tabular-nums'>{budget.used}</dd>
          </div>
          <div>
            <dt className='text-muted-foreground'>{t('userAdmin.reserved')}</dt>
            <dd className='tabular-nums'>{budget.reserved}</dd>
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
          className='bg-muted space-y-2 rounded-lg p-3'
          aria-live='polite'
        >
          <h3 className='font-medium'>{t('userAdmin.budgetPreview.title')}</h3>
          <p>
            {t(
              previewUnlimited
                ? 'userAdmin.budgetPreview.unlimited'
                : 'userAdmin.budgetPreview.remaining',
              { amount: remaining ?? t('userAdmin.budgetPreview.invalid') }
            )}
          </p>
          {!previewUnlimited && remaining !== null && (
            <p className='text-muted-foreground text-xs'>
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
          {budget.frozen && <p>{t('userAdmin.budgetPreview.frozen')}</p>}
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

export function PricingEditor({
  groupId,
  pricing,
  onStateChange,
}: {
  groupId: number
  pricing: GroupPricing
  onStateChange?: (state: EditorState) => void
}) {
  const { t } = useTranslation()
  const schema = z.object({
    multiplier: amountInput(t('userAdmin.multiplierHint'), true),
  })
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { multiplier: String(pricing.multiplier) },
    values: { multiplier: String(pricing.multiplier) },
    resetOptions: { keepDirtyValues: true },
  })
  const [saved, setSaved] = useState(false)
  const mutation = useAdminWrite(
    (value: z.infer<typeof schema>, token) =>
      gatewayUsersApi.setPricing(
        groupId,
        { multiplier: Number(value.multiplier) },
        token
      ),
    () => {
      setSaved(true)
      form.reset(form.getValues())
    }
  )
  const dirty = form.formState.isDirty
  const previewMultiplier = form.watch('multiplier')
  const example = multiplierPreview(previewMultiplier)
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
        className='space-y-3'
      >
        <TextField
          control={form.control}
          name='multiplier'
          label={t('userAdmin.multiplier')}
          type='number'
        />
        <p className='text-muted-foreground text-xs'>
          {t('userAdmin.pricingHint')}
        </p>
        <section
          className='bg-muted space-y-2 rounded-lg p-3'
          aria-live='polite'
        >
          <h3 className='font-medium'>
            {t('userAdmin.budgetPreview.multiplierTitle')}
          </h3>
          <p>
            {example
              ? t('userAdmin.budgetPreview.multiplierExample', {
                  multiplier: previewMultiplier,
                  ...example,
                })
              : t('userAdmin.budgetPreview.invalidMultiplier')}
          </p>
          <p className='text-muted-foreground text-xs'>
            {t('userAdmin.budgetPreview.rounding')}
          </p>
        </section>
        <WriteError error={mutation.error} />
        <div className='flex items-center gap-3'>
          <Button type='submit' size='sm' disabled={mutation.isPending}>
            {t(mutation.isPending ? 'userAdmin.saving' : 'userAdmin.save')}
          </Button>
          {saved && <span role='status'>{t('userAdmin.saved')}</span>}
        </div>
      </form>
    </Form>
  )
}

export function ActivationDialog({
  user,
  close,
}: {
  user: GatewayUser
  close: () => void
}) {
  const { t, i18n } = useTranslation()
  const { requestReauth } = useReauth()
  const [secret, setSecret] = useState<{
    code: string
    expiresAt: string
  } | null>(null)
  const [pending, setPending] = useState(false)
  const [failed, setFailed] = useState(false)
  const [copyStatus, setCopyStatus] = useState('')
  const alive = useRef(true)
  useEffect(() => {
    alive.current = true
    const unsubscribe = useAuthStore.subscribe((state, previous) => {
      if (
        previous.auth.sessionExpiresAt !== null &&
        state.auth.sessionExpiresAt === null
      ) {
        alive.current = false
        setSecret(null)
        close()
      }
    })
    return () => {
      alive.current = false
      unsubscribe()
    }
  }, [close])
  // Close marks any pending replay obsolete; no secret survives this surface.
  const dismiss = () => {
    alive.current = false
    setSecret(null)
    close()
  }
  const issue = async (token?: string) => {
    if (!alive.current) return
    setPending(true)
    setFailed(false)
    setSecret(null)
    try {
      const result = await gatewayUsersApi.activate(user.id, token)
      if (alive.current) setSecret(result)
    } catch (error) {
      if (isReauthRequired(error)) {
        if (token) throw error
        requestReauth({
          description: t('userAdmin.activationReauth'),
          replay: (confirmToken) => issue(confirmToken),
        })
      } else if (alive.current) setFailed(true)
    } finally {
      if (alive.current) setPending(false)
    }
  }
  const activationPackage = secret
    ? t('userAdmin.activationPackage.text', {
        entry: new URL('/user', window.location.origin).href,
        username: user.username,
        code: secret.code,
        expires: secret.expiresAt,
      })
    : ''
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !pending) dismiss()
      }}
    >
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>
            {t('userAdmin.activationTitle', {
              user: user.displayName || user.username,
            })}
          </DialogTitle>
          <DialogDescription>{t('userAdmin.activationHint')}</DialogDescription>
        </DialogHeader>
        {secret ? (
          <>
            <Input
              aria-label={t('userAdmin.activationCode')}
              value={secret.code}
              readOnly
              className='font-mono'
            />
            <p className='text-muted-foreground text-xs'>
              {t('userAdmin.expires', {
                time: new Date(secret.expiresAt).toLocaleString(
                  toBcp47(i18n.language)
                ),
              })}
            </p>
            <Textarea
              aria-label={t('userAdmin.activationPackage.label')}
              value={activationPackage}
              readOnly
              className='min-h-48'
            />
            <p className='text-muted-foreground text-xs'>
              {t('userAdmin.activationPackage.private')}
            </p>
            <Button
              onClick={() => {
                void copyText(activationPackage).then((ok) => {
                  if (alive.current)
                    setCopyStatus(
                      t(
                        ok
                          ? 'userAdmin.copied'
                          : 'userAdmin.activationPackage.copyFailed'
                      )
                    )
                })
              }}
            >
              {t('userAdmin.activationPackage.copy')}
            </Button>
            <Button
              variant='outline'
              onClick={() => {
                void copyText(secret.code).then((ok) => {
                  if (!alive.current) return
                  setCopyStatus(
                    t(ok ? 'userAdmin.copied' : 'userAdmin.copyFailed')
                  )
                })
              }}
            >
              {t('userAdmin.copyCode')}
            </Button>
            <p role='status'>{copyStatus}</p>
          </>
        ) : (
          <>
            <WriteError error={failed ? new Error('activation') : null} />
            <Button
              onClick={() => void issue()}
              disabled={pending || !user.enabled}
            >
              {t(pending ? 'userAdmin.saving' : 'userAdmin.issueCode')}
            </Button>
            {!user.enabled && (
              <Notice tone='warning'>
                {t('userAdmin.enableBeforeActivation')}
              </Notice>
            )}
          </>
        )}
        <Button variant='outline' onClick={dismiss} disabled={pending}>
          {t('userAdmin.close')}
        </Button>
      </DialogContent>
    </Dialog>
  )
}
