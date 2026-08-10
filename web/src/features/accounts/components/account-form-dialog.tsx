// metapi-go features/accounts/components — add/edit account form dialog.
//
// A Sheet side drawer with react-hook-form + zod, mirroring the keys
// feature's `api-keys-mutate-drawer.tsx` structure: schema factory from
// `lib/accounts-schema`, guarded one-time `form.reset` on open target,
// inert-until-initialized, and a footer submit button bound to the form id.
//
// On create success the form fires the guided "next step: configure routes"
// toast (account-created-toast.tsx) — step 2 of the site → account → route
// chain — rather than a plain confirmation.

import { zodResolver } from '@hookform/resolvers/zod'
import { Loader2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import {
  useForm,
  type SubmitErrorHandler,
  type UseFormReturn,
} from 'react-hook-form'
import { toast } from 'sonner'

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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'

import { useCreateAccount, useUpdateAccount } from '../api'
import { showAccountCreatedToast } from './account-created-toast'
import {
  getAccountFormDefaultValues,
  getAccountFormSchema,
  transformAccountToFormValues,
  transformFormToPayload,
  type AccountFormValues,
} from '../lib/accounts-schema'
import { type Account, type CredentialMode, type Site } from '../types'

interface AccountFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  mode: 'create' | 'edit'
  account?: Account | null
  sites: Site[]
}

export function AccountFormDialog({
  open,
  onOpenChange,
  mode,
  account,
  sites,
}: AccountFormDialogProps) {
  const createMutation = useCreateAccount()
  const updateMutation = useUpdateAccount()
  const isEdit = mode === 'edit' && !!account

  const schema = useMemo(() => getAccountFormSchema(), [])
  const form = useForm<AccountFormValues>({
    resolver: zodResolver(schema),
    defaultValues: getAccountFormDefaultValues(),
  })

  const credentialMode = form.watch('credentialMode') as CredentialMode
  const [initializedFor, setInitializedFor] = useState<string | null>(null)
  const isInitialized = initializedFor !== null

  // Guarded one-time reset per open target (create vs edit:<id>).
  useEffect(() => {
    if (!open) {
      setInitializedFor(null)
      return
    }
    const targetKey = isEdit && account ? `edit:${account.id}` : 'create'
    if (initializedFor === targetKey) return
    setInitializedFor(targetKey)
    const baseDefaults = getAccountFormDefaultValues(
      account?.credentialMode ?? 'session',
    )
    if (isEdit && account) {
      form.reset({ ...baseDefaults, ...transformAccountToFormValues(account) })
    } else {
      form.reset(baseDefaults)
    }
  }, [open, isEdit, account, initializedFor, form])

  const onSubmit = async (values: AccountFormValues) => {
    const payload = transformFormToPayload(values)
    try {
      if (isEdit && account) {
        await updateMutation.mutateAsync({ id: account.id, payload })
      } else {
        const result = await createMutation.mutateAsync(payload)
        const newId =
          result?.data?.id ?? result?.data?.account?.id ?? undefined
        showAccountCreatedToast(newId, values.siteId)
      }
      onOpenChange(false)
    } catch {
      // http-client already toasted the business/network error.
    }
  }

  const onInvalid: SubmitErrorHandler<AccountFormValues> = () => {
    toast.error('请检查表单标红字段')
  }

  const isSubmitting = createMutation.isPending || updateMutation.isPending
  const siteOptions = sites

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side='right' className='flex w-full flex-col gap-0 sm:max-w-lg'>
        <SheetHeader>
          <SheetTitle>{isEdit ? '编辑账号' : '添加账号'}</SheetTitle>
          <SheetDescription>
            {isEdit
              ? '更新账号凭证与配置。留空的凭证字段将保持不变。'
              : '连接一个站点账号：Session 模式用于签到/余额，API Key 模式用于代理转发。'}
          </SheetDescription>
        </SheetHeader>

        <Form {...form}>
          <form
            id='account-form'
            onSubmit={form.handleSubmit(onSubmit, onInvalid)}
            inert={!isInitialized ? true : undefined}
            aria-busy={!isInitialized}
            className='flex-1 space-y-5 overflow-y-auto p-4'
          >
            {/* Site selection */}
            <FormField
              control={form.control}
              name='siteId'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>站点</FormLabel>
                  <Select
                    value={field.value ? String(field.value) : ''}
                    onValueChange={(value) => field.onChange(Number(value))}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue placeholder='选择站点' />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      {siteOptions.length === 0 && (
                        <SelectItem value='__none' disabled>
                          暂无站点，请先添加站点
                        </SelectItem>
                      )}
                      {siteOptions.map((site) => (
                        <SelectItem key={site.id} value={String(site.id)}>
                          {site.name || site.url || `#${site.id}`}
                          {site.platform ? ` · ${site.platform}` : ''}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Credential mode toggle */}
            <FormItem>
              <FormLabel>凭证模式</FormLabel>
              <Tabs
                value={credentialMode}
                onValueChange={(value) =>
                  form.setValue('credentialMode', value as CredentialMode, {
                    shouldDirty: true,
                  })
                }
              >
                <TabsList>
                  <TabsTrigger value='session'>Session</TabsTrigger>
                  <TabsTrigger value='apikey'>API Key</TabsTrigger>
                </TabsList>
              </Tabs>
            </FormItem>

            {/* Connection name (optional) */}
            <FormField
              control={form.control}
              name='username'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>连接名称（可选）</FormLabel>
                  <FormControl>
                    <Input
                      placeholder='留空则使用站点用户名'
                      {...field}
                      value={field.value ?? ''}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {credentialMode === 'session' ? (
              <SessionFields form={form} />
            ) : (
              <ApiKeyFields form={form} />
            )}

            {/* Status */}
            <FormField
              control={form.control}
              name='status'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>状态</FormLabel>
                  <Select
                    value={field.value}
                    onValueChange={field.onChange}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value='active'>启用</SelectItem>
                      <SelectItem value='disabled'>禁用</SelectItem>
                      <SelectItem value='expired'>已过期</SelectItem>
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Checkin toggle */}
            <FormField
              control={form.control}
              name='checkinEnabled'
              render={({ field }) => (
                <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
                  <div className='space-y-0.5'>
                    <FormLabel>启用签到</FormLabel>
                    <FormDescription>每日自动签到维护余额</FormDescription>
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

            {/* Unit cost */}
            <FormField
              control={form.control}
              name='unitCost'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>单位成本（可选）</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      step='0.01'
                      placeholder='0.00'
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

            {/* Proxy URL */}
            <FormField
              control={form.control}
              name='proxyUrl'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>代理地址（可选）</FormLabel>
                  <FormControl>
                    <Input
                      placeholder='https://proxy.example.com'
                      {...field}
                      value={field.value ?? ''}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Tags */}
            <FormField
              control={form.control}
              name='tags'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>标签（可选）</FormLabel>
                  <FormControl>
                    <Input
                      placeholder='prod, priority'
                      {...field}
                      value={field.value ?? ''}
                    />
                  </FormControl>
                  <FormDescription>逗号分隔多个标签</FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </form>
        </Form>

        <SheetFooter>
          <Button
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={isSubmitting}
          >
            取消
          </Button>
          <Button
            type='submit'
            form='account-form'
            disabled={isSubmitting || !isInitialized}
          >
            {isSubmitting && <Loader2 className='animate-spin' />}
            {isEdit ? '保存修改' : '添加账号'}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

// ---------------------------------------------------------------------------
// Session-mode fields
// ---------------------------------------------------------------------------

interface SessionFieldsProps {
  form: UseFormReturn<AccountFormValues>
}

function SessionFields({ form }: SessionFieldsProps) {
  return (
    <>
      <FormField
        control={form.control}
        name='accessToken'
        render={({ field }) => (
          <FormItem>
            <FormLabel>Access Token / Cookie</FormLabel>
            <FormControl>
              <Textarea
                rows={4}
                placeholder='粘贴站点的 Session Token 或 Cookie'
                className='font-mono text-xs'
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormDescription>留空（编辑时）表示保持原有凭证</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={form.control}
        name='platformUserId'
        render={({ field }) => (
          <FormItem>
            <FormLabel>用户 ID（可选）</FormLabel>
            <FormControl>
              <Input
                type='number'
                placeholder='部分站点需要'
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

      <FormField
        control={form.control}
        name='refreshToken'
        render={({ field }) => (
          <FormItem>
            <FormLabel>Refresh Token（Sub2API 可选）</FormLabel>
            <FormControl>
              <Input
                className='font-mono text-xs'
                placeholder='留空将保持原有 refresh token'
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
        name='tokenExpiresAt'
        render={({ field }) => (
          <FormItem>
            <FormLabel>Token 过期时间戳（ms，可选）</FormLabel>
            <FormControl>
              <Input
                type='number'
                placeholder='毫秒时间戳'
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
    </>
  )
}

// ---------------------------------------------------------------------------
// API-Key-mode fields
// ---------------------------------------------------------------------------

function ApiKeyFields({ form }: SessionFieldsProps) {
  return (
    <>
      <FormField
        control={form.control}
        name='apiToken'
        render={({ field }) => (
          <FormItem>
            <FormLabel>API Key</FormLabel>
            <FormControl>
              <Textarea
                rows={3}
                placeholder='粘贴站点的 API Key（sk-...）'
                className='font-mono text-xs'
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
        name='skipModelFetch'
        render={({ field }) => (
          <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
            <div className='space-y-0.5'>
              <FormLabel>跳过模型获取</FormLabel>
              <FormDescription>不验证 Key 的可用模型（快速添加）</FormDescription>
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
    </>
  )
}
