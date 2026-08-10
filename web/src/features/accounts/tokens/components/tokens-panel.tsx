// metapi-go features/accounts/tokens/components — tokens sub-panel embedded
// inside the account detail sheet (not a standalone page, matching the
// legacy metapi design). Shows the account's tokens + an inline add/edit
// token form (RHF + Zod) + sync-from-site action.

import { zodResolver } from '@hookform/resolvers/zod'
import {
  CheckCircle2,
  Loader2,
  Pencil,
  Plus,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useForm, type SubmitErrorHandler } from 'react-hook-form'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'

import { type AccountToken } from '../../types'
import {
  useAccountTokens,
  useCreateAccountToken,
  useDeleteAccountToken,
  useSetDefaultAccountToken,
  useSyncAccountTokens,
  useToggleAccountTokenEnabled,
  useUpdateAccountToken,
} from '../api'
import {
  getAccountTokenFormDefaultValues,
  getAccountTokenFormSchema,
  transformTokenFormToPayload,
  type AccountTokenFormValues,
} from '../lib/tokens-schema'

interface TokensPanelProps {
  accountId: number
}

export function TokensPanel({ accountId }: TokensPanelProps) {
  const { data: tokens = [], isLoading } = useAccountTokens(accountId)
  const syncMutation = useSyncAccountTokens()
  const deleteMutation = useDeleteAccountToken()
  const setDefaultMutation = useSetDefaultAccountToken()
  const toggleEnabledMutation = useToggleAccountTokenEnabled()

  const [formOpen, setFormOpen] = useState(false)
  const [editingToken, setEditingToken] = useState<AccountToken | null>(null)

  const openCreateForm = () => {
    setEditingToken(null)
    setFormOpen(true)
  }

  const openEditForm = (token: AccountToken) => {
    setEditingToken(token)
    setFormOpen(true)
  }

  const closeForm = () => {
    setFormOpen(false)
    setEditingToken(null)
  }

  const handleSync = async () => {
    try {
      await syncMutation.mutateAsync(accountId)
    } catch {
      // http-client toasted
    }
  }

  return (
    <div className='flex flex-col gap-3'>
      <div className='flex items-center justify-between'>
        <h3 className='text-sm font-medium'>访问令牌</h3>
        <div className='flex items-center gap-1'>
          <Button
            variant='outline'
            size='xs'
            onClick={handleSync}
            disabled={syncMutation.isPending}
          >
            {syncMutation.isPending ? (
              <Loader2 className='animate-spin' />
            ) : (
              <RefreshCw />
            )}
            同步站点令牌
          </Button>
          <Button size='xs' onClick={openCreateForm}>
            <Plus />
            添加
          </Button>
        </div>
      </div>

      {formOpen && (
        <AccountTokenForm
          accountId={accountId}
          token={editingToken}
          onClose={closeForm}
        />
      )}

      <Separator />

      {isLoading ? (
        <div className='flex items-center justify-center py-6 text-muted-foreground text-sm'>
          <Loader2 className='size-4 animate-spin' />
          加载令牌…
        </div>
      ) : tokens.length === 0 ? (
        <p className='py-6 text-center text-muted-foreground text-sm'>
          暂无令牌。点击「添加」手动新增，或「同步站点令牌」从站点拉取。
        </p>
      ) : (
        <ul className='flex flex-col divide-y rounded-lg border'>
          {tokens.map((token) => (
            <TokenRow
              key={token.id}
              token={token}
              onEdit={() => openEditForm(token)}
              onDelete={() => deleteMutation.mutate(token.id)}
              onSetDefault={() => setDefaultMutation.mutate(token.id)}
              onToggleEnabled={(enabled) =>
                toggleEnabledMutation.mutate({ id: token.id, enabled })
              }
              isDeleting={deleteMutation.isPending}
              isToggling={toggleEnabledMutation.isPending}
            />
          ))}
        </ul>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Token row
// ---------------------------------------------------------------------------

interface TokenRowProps {
  token: AccountToken
  onEdit: () => void
  onDelete: () => void
  onSetDefault: () => void
  onToggleEnabled: (enabled: boolean) => void
  isDeleting: boolean
  isToggling: boolean
}

function TokenRow({
  token,
  onEdit,
  onDelete,
  onSetDefault,
  onToggleEnabled,
  isDeleting,
  isToggling,
}: TokenRowProps) {
  const isMaskedPending = token.valueStatus === 'masked_pending'
  return (
    <li className='flex items-center gap-3 px-3 py-2'>
      <div className='flex min-w-0 flex-1 flex-col gap-0.5'>
        <div className='flex items-center gap-1.5'>
          <span className='truncate text-sm font-medium'>
            {token.name || '未命名令牌'}
          </span>
          {token.isDefault && (
            <Badge variant='default' className='text-[10px]'>
              默认
            </Badge>
          )}
          {isMaskedPending && (
            <Badge variant='warning' className='text-[10px]'>
              待补全
            </Badge>
          )}
        </div>
        <span className='font-mono text-[11px] text-muted-foreground truncate'>
          {token.tokenMasked || token.token || '—'}
        </span>
      </div>

      <div className='flex items-center gap-1'>
        {!token.isDefault && (
          <Button
            variant='ghost'
            size='icon-sm'
            onClick={onSetDefault}
            title='设为默认'
            aria-label='设为默认'
          >
            <CheckCircle2 />
          </Button>
        )}
        <Switch
          checked={token.enabled ?? false}
          onCheckedChange={onToggleEnabled}
          disabled={isToggling}
          aria-label='启用/禁用令牌'
        />
        <Button
          variant='ghost'
          size='icon-sm'
          onClick={onEdit}
          title='编辑'
          aria-label='编辑'
        >
          <Pencil />
        </Button>
        <Button
          variant='ghost'
          size='icon-sm'
          onClick={onDelete}
          disabled={isDeleting}
          className={cn('text-muted-foreground hover:text-destructive')}
          title='删除'
          aria-label='删除'
        >
          <Trash2 />
        </Button>
      </div>
    </li>
  )
}

// ---------------------------------------------------------------------------
// Inline add/edit token form
// ---------------------------------------------------------------------------

interface AccountTokenFormProps {
  accountId: number
  token: AccountToken | null
  onClose: () => void
}

function AccountTokenForm({
  accountId,
  token,
  onClose,
}: AccountTokenFormProps) {
  const isEdit = !!token
  const createMutation = useCreateAccountToken()
  const updateMutation = useUpdateAccountToken()

  const schema = useMemo(() => getAccountTokenFormSchema(), [])
  const form = useForm<AccountTokenFormValues>({
    resolver: zodResolver(schema),
    defaultValues: getAccountTokenFormDefaultValues(accountId),
  })

  const unlimited = form.watch('unlimited')

  useEffect(() => {
    if (token) {
      form.reset({
        accountId,
        name: token.name || '',
        token: '',
        tokenGroup: token.tokenGroup ?? 'default',
        quota: undefined,
        unlimited: true,
        expiresAt: '',
        allowedIps: '',
      })
    } else {
      form.reset(getAccountTokenFormDefaultValues(accountId))
    }
  }, [token, accountId, form])

  const onSubmit = async (values: AccountTokenFormValues) => {
    const payload = transformTokenFormToPayload(values)
    try {
      if (isEdit && token) {
        await updateMutation.mutateAsync({
          id: token.id,
          payload: {
            name: payload.name,
            tokenGroup: payload.tokenGroup,
            quota: payload.quota,
            remainQuota: payload.remainQuota,
            unlimitedQuota: payload.unlimitedQuota,
            expiredTime: payload.expiredTime,
            allowedIps: payload.allowedIps,
            // token value only sent when the operator re-enters it
            ...(values.token ? { token: values.token } : {}),
          },
        })
      } else {
        await createMutation.mutateAsync(payload)
      }
      onClose()
    } catch {
      // http-client toasted
    }
  }

  const onInvalid: SubmitErrorHandler<AccountTokenFormValues> = () => {
    toast.error('请检查令牌表单')
  }

  const isSubmitting = createMutation.isPending || updateMutation.isPending

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit, onInvalid)}
        className='flex flex-col gap-3 rounded-lg border bg-muted/30 p-3'
      >
        <FormField
          control={form.control}
          name='name'
          render={({ field }) => (
            <FormItem>
              <FormLabel>令牌名称</FormLabel>
              <FormControl>
                <Input
                  placeholder='生产令牌'
                  {...field}
                  value={field.value ?? ''}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name='token'
          render={({ field }) => (
            <FormItem>
              <FormLabel>令牌值</FormLabel>
              <FormControl>
                <Input
                  className='font-mono text-xs'
                  placeholder={isEdit ? '留空保持不变' : 'sk-...'}
                  {...field}
                  value={field.value ?? ''}
                />
              </FormControl>
              <FormDescription>
                {isEdit ? '编辑时留空表示不修改令牌值' : undefined}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />

        <div className='grid grid-cols-2 gap-3'>
          <FormField
            control={form.control}
            name='tokenGroup'
            render={({ field }) => (
              <FormItem>
                <FormLabel>分组</FormLabel>
                <FormControl>
                  <Input
                    placeholder='default'
                    {...field}
                    value={field.value ?? ''}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='quota'
            render={({ field }) => (
              <FormItem>
                <FormLabel>额度</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    placeholder='不限'
                    disabled={unlimited}
                    value={field.value ?? ''}
                    onChange={(event) =>
                      field.onChange(
                        event.target.value === ''
                          ? undefined
                          : Number(event.target.value),
                      )
                    }
                    onBlur={field.onBlur}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>

        <FormField
          control={form.control}
          name='unlimited'
          render={({ field }) => (
            <FormItem className='flex flex-row items-center justify-between rounded-lg border p-2.5'>
              <div className='space-y-0.5'>
                <FormLabel>不限额度</FormLabel>
                <FormDescription>勾选后忽略额度字段</FormDescription>
              </div>
              <FormControl>
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              </FormControl>
            </FormItem>
          )}
        />

        <div className='grid grid-cols-2 gap-3'>
          <FormField
            control={form.control}
            name='expiresAt'
            render={({ field }) => (
              <FormItem>
                <FormLabel>过期时间（可选）</FormLabel>
                <FormControl>
                  <Input
                    type='datetime-local'
                    {...field}
                    value={field.value ?? ''}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='allowedIps'
            render={({ field }) => (
              <FormItem>
                <FormLabel>IP 白名单（可选）</FormLabel>
                <FormControl>
                  <Input
                    placeholder='逗号分隔'
                    {...field}
                    value={field.value ?? ''}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>

        <div className='flex justify-end gap-2'>
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={onClose}
            disabled={isSubmitting}
          >
            取消
          </Button>
          <Button type='submit' size='sm' disabled={isSubmitting}>
            {isSubmitting && <Loader2 className='animate-spin' />}
            {isEdit ? '保存' : '添加令牌'}
          </Button>
        </div>
      </form>
    </Form>
  )
}
