// metapi-go features/token-routes/components — add/edit route form dialog.
//
// A Sheet side drawer with react-hook-form + zod, mirroring the accounts
// feature's `account-form-dialog.tsx` structure: schema factory from
// `lib/routes-schema`, guarded one-time `form.reset` on open target,
// inert-until-initialized, and a footer submit button bound to the form id.
//
// The form is mode-aware (pattern vs explicit_group). Pattern mode collects a
// model matching rule (exact name or `re:` regex) + optional channel drafts
// (a checkbox list of accounts that have tokens for the pattern). Explicit
// group mode collects a display name + a multi-select of existing exact-model
// routes as the group's source routes.
//
// On create success the form fires the guided "configuration complete" toast
// (route-completion-toast.tsx) — the final step of the site → account →
// route chain — rather than a plain confirmation. Channel drafts are forwarded
// to `api.batchAddChannels` after the route is created/updated.

import { zodResolver } from '@hookform/resolvers/zod'
import { Loader2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import {
  useForm,
  type SubmitErrorHandler,
} from 'react-hook-form'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'

import {
  type BatchAddChannelsResult,
  useBatchAddChannels,
  useCreateRoute,
  useUpdateRoute,
} from '../api'
import { showRouteCompletionToast } from './route-completion-toast'
import {
  getRouteFormDefaultValues,
  getRouteFormSchema,
  transformFormToPayload,
  transformRouteToFormValues,
  type RouteFormValues,
} from '../lib/routes-schema'
import {
  type RouteMode,
  type RouteRoutingStrategy,
  type RouteSummaryRow,
} from '../types'
import { getModelPatternError, isRegexModelPattern } from '../utils'

// ---------------------------------------------------------------------------
// Account option — the channel-draft picker source. The page computes this
// from the model token candidates endpoint, filtered by the form's current
// model pattern, so the operator only sees accounts that actually carry a
// matching token.
// ---------------------------------------------------------------------------

export type RouteAccountOption = {
  id: number
  label: string
}

interface RouteFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  mode: 'create' | 'edit'
  route?: RouteSummaryRow | null
  /**
   * Exact-model routes available as source routes for explicit_group mode.
   * Excludes the current route (on edit) and any existing group routes.
   */
  availableRoutes: RouteSummaryRow[]
  /**
   * Accounts available as channel drafts for pattern mode. Filtered by the
   * page to those carrying a token matching the form's current model pattern.
   */
  accountOptions: RouteAccountOption[]
  /**
   * Chain context carried from the accounts page deep-link
   * (?accountId=&siteId=) — forwarded to the completion toast so the operator
   * can confirm which account/site the new route serves.
   */
  chainContext?: { accountId?: number; siteId?: number }
}

export function RouteFormDialog({
  open,
  onOpenChange,
  mode,
  route,
  availableRoutes,
  accountOptions,
  chainContext,
}: RouteFormDialogProps) {
  const createMutation = useCreateRoute()
  const updateMutation = useUpdateRoute()
  const batchAddChannelsMutation = useBatchAddChannels()
  const isEdit = mode === 'edit' && !!route

  const schema = useMemo(() => getRouteFormSchema(), [])
  const form = useForm<RouteFormValues>({
    resolver: zodResolver(schema),
    defaultValues: getRouteFormDefaultValues(),
  })

  const routeMode = form.watch('routeMode') as RouteMode
  const modelPattern = form.watch('modelPattern') ?? ''
  const [initializedFor, setInitializedFor] = useState<string | null>(null)
  const isInitialized = initializedFor !== null

  // Guarded one-time reset per open target (create vs edit:<id>).
  useEffect(() => {
    if (!open) {
      setInitializedFor(null)
      return
    }
    const targetKey = isEdit && route ? `edit:${route.id}` : 'create'
    if (initializedFor === targetKey) return
    setInitializedFor(targetKey)
    const baseDefaults = getRouteFormDefaultValues(
      route?.routeMode === 'explicit_group' ? 'explicit_group' : 'pattern',
    )
    if (isEdit && route) {
      form.reset({
        ...baseDefaults,
        ...transformRouteToFormValues(route),
      })
    } else {
      form.reset(baseDefaults)
    }
  }, [open, isEdit, route, initializedFor, form])

  const patternError = useMemo(
    () => (modelPattern ? getModelPatternError(modelPattern) : null),
    [modelPattern],
  )

  const onSubmit = async (values: RouteFormValues) => {
    const payload = transformFormToPayload(values)
    const drafts = (values.channelDrafts ?? []).filter(
      (draft) => draft.accountId > 0,
    )
    try {
      let routeId: number | undefined
      if (isEdit && route) {
        await updateMutation.mutateAsync({ id: route.id, payload })
        routeId = route.id
      } else {
        const result = await createMutation.mutateAsync(payload)
        routeId = result?.data?.id
      }

      if (routeId && drafts.length > 0) {
        await batchAddChannelsMutation.mutateAsync({
          routeId,
          channels: drafts,
        })
      }

      if (!isEdit) {
        showRouteCompletionToast(routeId, chainContext)
      }
      onOpenChange(false)
    } catch {
      // http-client already toasted the business/network error.
    }
  }

  const onInvalid: SubmitErrorHandler<RouteFormValues> = () => {
    toast.error('请检查表单标红字段')
  }

  const isSubmitting =
    createMutation.isPending ||
    updateMutation.isPending ||
    batchAddChannelsMutation.isPending

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side='right'
        className='flex w-full flex-col gap-0 sm:max-w-lg'
      >
        <SheetHeader>
          <SheetTitle>{isEdit ? '编辑路由' : '添加路由'}</SheetTitle>
          <SheetDescription>
            {isEdit
              ? '更新路由匹配规则与配置。留空的可选字段将保持不变。'
              : '配置一条路由：匹配模式按模型名/正则匹配，分组模式聚合多个精确路由。'}
          </SheetDescription>
        </SheetHeader>

        <Form {...form}>
          <form
            id='route-form'
            onSubmit={form.handleSubmit(onSubmit, onInvalid)}
            inert={!isInitialized ? true : undefined}
            aria-busy={!isInitialized}
            className='flex-1 space-y-5 overflow-y-auto p-4'
          >
            {/* Route mode toggle */}
            <FormItem>
              <FormLabel>路由类型</FormLabel>
              <Tabs
                value={routeMode}
                onValueChange={(value) =>
                  form.setValue('routeMode', value as RouteMode, {
                    shouldDirty: true,
                  })
                }
              >
                <TabsList>
                  <TabsTrigger value='explicit_group'>分组</TabsTrigger>
                  <TabsTrigger value='pattern'>匹配</TabsTrigger>
                </TabsList>
              </Tabs>
              <FormDescription>
                {routeMode === 'explicit_group'
                  ? '分组：对外暴露一个模型名，聚合多个精确路由作为来源。'
                  : '匹配：按模型名或 re: 正则匹配上游请求。'}
              </FormDescription>
            </FormItem>

            {routeMode === 'pattern' ? (
              <PatternModeFields
                form={form}
                patternError={patternError}
                accountOptions={accountOptions}
              />
            ) : (
              <GroupModeFields form={form} availableRoutes={availableRoutes} />
            )}

            {/* Display icon */}
            <FormField
              control={form.control}
              name='displayIcon'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>显示图标（可选）</FormLabel>
                  <FormControl>
                    <Input
                      placeholder='留空=自动品牌；brand:openai；__route_icon_none__=无'
                      {...field}
                      value={field.value ?? ''}
                    />
                  </FormControl>
                  <FormDescription>
                    支持 `brand:&lt;key&gt;` 品牌图标，或 `__route_icon_none__` 禁用图标。
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Context length */}
            <FormField
              control={form.control}
              name='contextLength'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>上下文长度（可选）</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      placeholder='128000'
                      {...field}
                      value={field.value ?? ''}
                    />
                  </FormControl>
                  <FormDescription>
                    路由上下文窗口（tokens）。留空或 0 = 未知，不强制。
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Routing strategy */}
            <FormField
              control={form.control}
              name='routingStrategy'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>路由策略</FormLabel>
                  <Select
                    value={field.value ?? 'weighted'}
                    onValueChange={(value) =>
                      field.onChange(value as RouteRoutingStrategy)
                    }
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value='weighted'>权重随机</SelectItem>
                      <SelectItem value='round_robin'>轮询</SelectItem>
                      <SelectItem value='stable_first'>稳定优先</SelectItem>
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    多通道时的选中策略：权重随机（默认）、轮询、稳定优先。
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            {/* Model mapping */}
            <FormField
              control={form.control}
              name='modelMapping'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>模型映射（可选）</FormLabel>
                  <FormControl>
                    <Textarea
                      placeholder='{"gpt-4":"gpt-4o"} 或留空'
                      rows={2}
                      {...field}
                      value={field.value ?? ''}
                    />
                  </FormControl>
                  <FormDescription>
                    将匹配的模型名映射为上游模型名（JSON 对象）。
                  </FormDescription>
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
            form='route-form'
            disabled={isSubmitting || !isInitialized}
          >
            {isSubmitting && <Loader2 className='animate-spin' />}
            {isEdit ? '保存修改' : '添加路由'}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

// ---------------------------------------------------------------------------
// Pattern mode fields — model matching rule + channel drafts
// ---------------------------------------------------------------------------

function PatternModeFields({
  form,
  patternError,
  accountOptions,
}: {
  form: ReturnType<typeof useForm<RouteFormValues>>
  patternError: string | null
  accountOptions: RouteAccountOption[]
}) {
  const modelPattern = form.watch('modelPattern') ?? ''
  const isRegex = isRegexModelPattern(modelPattern)

  return (
    <>
      <FormField
        control={form.control}
        name='modelPattern'
        render={({ field }) => (
          <FormItem>
            <FormLabel>模型匹配规则</FormLabel>
            <FormControl>
              <Input
                placeholder='gpt-4o-mini 或 re:^claude-.*$'
                className='font-mono'
                aria-invalid={Boolean(patternError)}
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormDescription>
              {isRegex
                ? '当前为正则匹配（re: 前缀）。'
                : '当前为精确模型匹配。正则请使用 re: 前缀。'}
            </FormDescription>
            {patternError && (
              <p className='text-sm text-destructive'>{patternError}</p>
            )}
            <FormMessage />
          </FormItem>
        )}
      />

      {/* Channel drafts — checkbox list of accounts carrying matching tokens */}
      {accountOptions.length > 0 && (
        <FormField
          control={form.control}
          name='channelDrafts'
          render={({ field }) => {
            const selected = field.value ?? []
            const selectedIds = new Set(
              selected.map((draft) => draft.accountId),
            )
            const toggleAccount = (accountId: number, checked: boolean) => {
              if (checked) {
                field.onChange([
                  ...selected,
                  { accountId },
                ])
              } else {
                field.onChange(
                  selected.filter((draft) => draft.accountId !== accountId),
                )
              }
            }
            return (
              <FormItem>
                <FormLabel>通道（可选）</FormLabel>
                <FormDescription>
                  勾选账号作为该路由的通道，创建后将自动绑定。
                </FormDescription>
                <div className='max-h-48 space-y-1 overflow-y-auto rounded-lg border p-2'>
                  {accountOptions.map((account) => (
                    <label
                      key={account.id}
                      className='flex items-center gap-2 rounded px-2 py-1 hover:bg-muted'
                    >
                      <Checkbox
                        checked={selectedIds.has(account.id)}
                        onCheckedChange={(value) =>
                          toggleAccount(account.id, Boolean(value))
                        }
                      />
                      <span className='text-sm truncate'>{account.label}</span>
                    </label>
                  ))}
                </div>
                <FormMessage />
              </FormItem>
            )
          }}
        />
      )}
    </>
  )
}

// ---------------------------------------------------------------------------
// Group mode fields — display name + source route multi-select
// ---------------------------------------------------------------------------

function GroupModeFields({
  form,
  availableRoutes,
}: {
  form: ReturnType<typeof useForm<RouteFormValues>>
  availableRoutes: RouteSummaryRow[]
}) {
  return (
    <>
      <FormField
        control={form.control}
        name='displayName'
        render={({ field }) => (
          <FormItem>
            <FormLabel>对外模型名</FormLabel>
            <FormControl>
              <Input
                placeholder='group-claude-opus'
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormDescription>
              客户端请求时使用的模型名，由分组下的来源路由实际响应。
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={form.control}
        name='sourceRouteIds'
        render={({ field }) => {
          const selected = field.value ?? []
          const selectedSet = new Set(selected)
          const toggleRoute = (routeId: number, checked: boolean) => {
            if (checked) {
              field.onChange([...selected, routeId])
            } else {
              field.onChange(
                selected.filter((id) => id !== routeId),
              )
            }
          }
          return (
            <FormItem>
              <FormLabel>来源路由</FormLabel>
              <FormDescription>
                选择该分组聚合的精确模型路由（至少一个）。
              </FormDescription>
              <div className='max-h-56 space-y-1 overflow-y-auto rounded-lg border p-2'>
                {availableRoutes.length === 0 && (
                  <p className='px-2 py-4 text-center text-sm text-muted-foreground'>
                    暂无可用的精确路由，请先创建匹配模式路由。
                  </p>
                )}
                {availableRoutes.map((route) => (
                  <label
                    key={route.id}
                    className='flex items-center gap-2 rounded px-2 py-1 hover:bg-muted'
                  >
                    <Checkbox
                      checked={selectedSet.has(route.id)}
                      onCheckedChange={(value) =>
                        toggleRoute(route.id, Boolean(value))
                      }
                    />
                    <span className='text-sm font-mono truncate'>
                      {route.modelPattern}
                    </span>
                  </label>
                ))}
              </div>
              <FormMessage />
            </FormItem>
          )
        }}
      />
    </>
  )
}

// ---------------------------------------------------------------------------
// Helpers exported for the page — interpret batchAddChannels results into a
// single summary toast (called from the form submit when drafts are present).
// ---------------------------------------------------------------------------

export function describeBatchAddChannelsResult(
  result: BatchAddChannelsResult | undefined,
): string {
  if (!result) return '通道已添加'
  const parts: string[] = [`已添加 ${result.created ?? 0} 个通道`]
  if ((result.skipped ?? 0) > 0) parts.push(`跳过 ${result.skipped} 个重复`)
  if ((result.errors?.length ?? 0) > 0) {
    parts.push(`${result.errors?.length ?? 0} 个错误`)
  }
  return parts.join('，')
}
