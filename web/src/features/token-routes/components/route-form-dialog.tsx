/* eslint-disable react/only-export-components -- dialog component co-located with exported type */
// metapi-go features/token-routes/components — add/edit route form dialog.
// i18n: all user-visible strings migrated to t() calls.
// `getModelPatternError()` returns pre-translated strings via i18n.t().

import { zodResolver } from '@hookform/resolvers/zod'
import { Loader2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useForm, type SubmitErrorHandler } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
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
import { toast } from '@/lib/toast'

import { useBatchAddChannels, useCreateRoute, useUpdateRoute } from '../api'
import {
  buildChannelDraftSeed,
  getRouteFormDefaultValues,
  getRouteFormSchema,
  transformFormToPayload,
  transformRouteToFormValues,
  type RouteFormValues,
} from '../lib/routes-schema'
import type { RouteMode, RouteRoutingStrategy, RouteSummaryRow } from '../types'
import { getModelPatternError, isRegexModelPattern } from '../utils'
import { showRouteCompletionToast } from './route-completion-toast'

export type RouteAccountOption = { id: number; label: string }

interface RouteFormDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  mode: 'create' | 'edit'
  route?: RouteSummaryRow | null
  availableRoutes: RouteSummaryRow[]
  accountOptions: RouteAccountOption[]
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
  const { t } = useTranslation()
  const createMutation = useCreateRoute()
  const updateMutation = useUpdateRoute()
  const batchAddChannelsMutation = useBatchAddChannels()
  const isEdit = mode === 'edit' && !!route

  const schema = useMemo(() => getRouteFormSchema(), [])
  const form = useForm<RouteFormValues>({
    resolver: zodResolver(schema),
    defaultValues: getRouteFormDefaultValues(),
  })
  const { handleOpenChange, guard } = useDirtyDialogClose({
    enabled: form.formState.isDirty,
    onDiscard: () => form.reset(),
    onOpenChange,
  })
  const routeMode = form.watch('routeMode') as RouteMode
  const modelPattern = form.watch('modelPattern') ?? ''
  const [initializedFor, setInitializedFor] = useState<string | null>(null)
  const isInitialized = initializedFor !== null

  useEffect(() => {
    if (!open) {
      setInitializedFor(null)
      return
    }
    const targetKey = isEdit && route ? `edit:${route.id}` : 'create'
    if (initializedFor === targetKey) return
    setInitializedFor(targetKey)
    const baseDefaults = getRouteFormDefaultValues(
      route?.routeMode === 'explicit_group' ? 'explicit_group' : 'pattern'
    )
    if (isEdit && route) {
      form.reset({ ...baseDefaults, ...transformRouteToFormValues(route) })
    } else {
      form.reset({
        ...baseDefaults,
        channelDrafts: buildChannelDraftSeed(chainContext?.accountId),
      })
    }
  }, [open, isEdit, route, initializedFor, chainContext, form])

  const patternError = useMemo(
    () => (modelPattern ? getModelPatternError(modelPattern) : null),
    [modelPattern]
  )

  const onSubmit = async (values: RouteFormValues) => {
    const payload = transformFormToPayload(values)
    const drafts = (values.channelDrafts ?? []).filter(
      (draft) => draft.accountId > 0
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
      form.reset()
      onOpenChange(false)
    } catch {}
  }

  const onInvalid: SubmitErrorHandler<RouteFormValues> = () => {
    toast.error(t('tokenRoutes.form.invalid'))
  }
  const isSubmitting =
    createMutation.isPending ||
    updateMutation.isPending ||
    batchAddChannelsMutation.isPending

  return (
    <Sheet open={open} onOpenChange={handleOpenChange}>
      <SheetContent
        side='right'
        className='flex w-full flex-col gap-0 sm:max-w-lg'
      >
        <SheetHeader>
          <SheetTitle>
            {isEdit
              ? t('tokenRoutes.form.editTitle')
              : t('tokenRoutes.form.addTitle')}
          </SheetTitle>
          <SheetDescription>
            {isEdit
              ? t('tokenRoutes.form.editDescription')
              : t('tokenRoutes.form.addDescription')}
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
            <FormItem>
              <FormLabel>{t('tokenRoutes.form.routeType')}</FormLabel>
              <Tabs
                value={routeMode}
                onValueChange={(value) =>
                  form.setValue('routeMode', value as RouteMode, {
                    shouldDirty: true,
                  })
                }
              >
                <TabsList>
                  <TabsTrigger value='explicit_group'>
                    {t('tokenRoutes.form.modeGroup')}
                  </TabsTrigger>
                  <TabsTrigger value='pattern'>
                    {t('tokenRoutes.form.modePattern')}
                  </TabsTrigger>
                </TabsList>
              </Tabs>
              <FormDescription>
                {routeMode === 'explicit_group'
                  ? t('tokenRoutes.form.modeGroupHint')
                  : t('tokenRoutes.form.modePatternHint')}
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
            <FormField
              control={form.control}
              name='displayIcon'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('tokenRoutes.form.displayIcon')}</FormLabel>
                  <FormControl>
                    <Input
                      placeholder={t('tokenRoutes.form.displayIconPlaceholder')}
                      {...field}
                      value={field.value ?? ''}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('tokenRoutes.form.displayIconHint')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='contextLength'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('tokenRoutes.form.contextLength')}</FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      placeholder='128000'
                      {...field}
                      value={field.value ?? ''}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('tokenRoutes.form.contextLengthHint')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='routingStrategy'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('tokenRoutes.form.routingStrategy')}</FormLabel>
                  <Select
                    value={field.value ?? 'weighted'}
                    onValueChange={(value) =>
                      field.onChange(value as RouteRoutingStrategy)
                    }
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue>
                          {(selected) =>
                            t(
                              `tokenRoutes.strategies.${String(selected ?? 'weighted')}`
                            )
                          }
                        </SelectValue>
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value='weighted'>
                        {t('tokenRoutes.strategies.weighted')}
                      </SelectItem>
                      <SelectItem value='round_robin'>
                        {t('tokenRoutes.strategies.round_robin')}
                      </SelectItem>
                      <SelectItem value='stable_first'>
                        {t('tokenRoutes.strategies.stable_first')}
                      </SelectItem>
                      <SelectItem value='least_busy'>
                        {t('tokenRoutes.strategies.least_busy')}
                      </SelectItem>
                      <SelectItem value='lowest_latency'>
                        {t('tokenRoutes.strategies.lowest_latency')}
                      </SelectItem>
                      <SelectItem value='lowest_cost'>
                        {t('tokenRoutes.strategies.lowest_cost')}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    {t('tokenRoutes.form.routingStrategyHint')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='modelMapping'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('tokenRoutes.form.modelMapping')}</FormLabel>
                  <FormControl>
                    <Textarea
                      placeholder={t(
                        'tokenRoutes.form.modelMappingPlaceholder'
                      )}
                      rows={2}
                      {...field}
                      value={field.value ?? ''}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('tokenRoutes.form.modelMappingHint')}
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
            {t('common.cancel')}
          </Button>
          <Button
            type='submit'
            form='route-form'
            disabled={isSubmitting || !isInitialized}
          >
            {isSubmitting && <Loader2 className='animate-spin' />}
            {isEdit ? t('tokenRoutes.form.save') : t('tokenRoutes.form.create')}
          </Button>
        </SheetFooter>
      </SheetContent>
      {guard}
    </Sheet>
  )
}

function PatternModeFields({
  form,
  patternError,
  accountOptions,
}: {
  form: ReturnType<typeof useForm<RouteFormValues>>
  patternError: string | null
  accountOptions: RouteAccountOption[]
}) {
  const { t } = useTranslation()
  const modelPattern = form.watch('modelPattern') ?? ''
  const isRegex = isRegexModelPattern(modelPattern)
  return (
    <>
      <FormField
        control={form.control}
        name='modelPattern'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('tokenRoutes.formPattern.modelRule')}</FormLabel>
            <FormControl>
              <Input
                placeholder={t('tokenRoutes.formPattern.modelRulePlaceholder')}
                className='font-mono'
                aria-invalid={Boolean(patternError)}
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormDescription>
              {isRegex
                ? t('tokenRoutes.formPattern.regexHint')
                : t('tokenRoutes.formPattern.exactHint')}
            </FormDescription>
            {patternError && (
              <p className='text-destructive text-sm'>{patternError}</p>
            )}
            <FormMessage />
          </FormItem>
        )}
      />
      {accountOptions.length > 0 && (
        <FormField
          control={form.control}
          name='channelDrafts'
          render={({ field }) => {
            const selected = field.value ?? []
            const selectedIds = new Set(
              selected.map((draft) => draft.accountId)
            )
            const toggleAccount = (accountId: number, checked: boolean) => {
              if (checked) {
                field.onChange([...selected, { accountId }])
              } else {
                field.onChange(
                  selected.filter((draft) => draft.accountId !== accountId)
                )
              }
            }
            return (
              <FormItem>
                <FormLabel>{t('tokenRoutes.formPattern.channels')}</FormLabel>
                <FormDescription>
                  {t('tokenRoutes.formPattern.channelsHint')}
                </FormDescription>
                <div className='max-h-48 space-y-1 overflow-y-auto rounded-lg border p-2'>
                  {accountOptions.map((account) => (
                    <label
                      key={account.id}
                      className='hover:bg-muted flex items-center gap-2 rounded px-2 py-1'
                    >
                      <Checkbox
                        checked={selectedIds.has(account.id)}
                        onCheckedChange={(value) =>
                          toggleAccount(account.id, Boolean(value))
                        }
                      />
                      <span className='truncate text-sm'>{account.label}</span>
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

function GroupModeFields({
  form,
  availableRoutes,
}: {
  form: ReturnType<typeof useForm<RouteFormValues>>
  availableRoutes: RouteSummaryRow[]
}) {
  const { t } = useTranslation()
  return (
    <>
      <FormField
        control={form.control}
        name='displayName'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('tokenRoutes.formGroup.displayName')}</FormLabel>
            <FormControl>
              <Input
                placeholder={t('tokenRoutes.formGroup.displayNamePlaceholder')}
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormDescription>
              {t('tokenRoutes.formGroup.displayNameHint')}
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
              field.onChange(selected.filter((id) => id !== routeId))
            }
          }
          return (
            <FormItem>
              <FormLabel>{t('tokenRoutes.formGroup.sourceRoutes')}</FormLabel>
              <FormDescription>
                {t('tokenRoutes.formGroup.sourceRoutesHint')}
              </FormDescription>
              <div className='max-h-56 space-y-1 overflow-y-auto rounded-lg border p-2'>
                {availableRoutes.length === 0 && (
                  <p className='text-muted-foreground px-2 py-4 text-center text-sm'>
                    {t('tokenRoutes.formGroup.sourceRoutesEmpty')}
                  </p>
                )}
                {availableRoutes.map((route) => (
                  <label
                    key={route.id}
                    className='hover:bg-muted flex items-center gap-2 rounded px-2 py-1'
                  >
                    <Checkbox
                      checked={selectedSet.has(route.id)}
                      onCheckedChange={(value) =>
                        toggleRoute(route.id, Boolean(value))
                      }
                    />
                    <span className='truncate font-mono text-sm'>
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
