/* eslint-disable react/only-export-components -- dialog component co-located with exported type */
// metapi-go features/token-routes/components — add/edit route form dialog.
// i18n: all user-visible strings migrated to t() calls.
// `getModelPatternError()` returns pre-translated strings via i18n.t().

import { zodResolver } from '@hookform/resolvers/zod'
import { ChevronDown, Search } from 'lucide-react'
import { useEffect, useId, useMemo, useState } from 'react'
import { useForm, type SubmitErrorHandler } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
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
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toast } from '@/lib/toast'

import {
  resolveCreatedRouteId,
  useBatchAddChannels,
  useCreateRoute,
  useUpdateRoute,
} from '../api'
import {
  buildChannelDraftSeed,
  getRouteFormDefaultValues,
  getRouteFormSchema,
  setChannelDraftSelection,
  transformFormToPayload,
  transformRouteToFormValues,
  type RouteFormValues,
} from '../lib/routes-schema'
import type { RouteMode, RouteSummaryRow } from '../types'
import { getModelPatternError, isRegexModelPattern } from '../utils'
import { RouteAdvancedFields } from './route-advanced-fields'
import { RouteChannelEditor } from './route-channel-editor'
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
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const isInitialized = initializedFor !== null

  useEffect(() => {
    if (!open) {
      setInitializedFor(null)
      return
    }
    const targetKey = isEdit && route ? `edit:${route.id}` : 'create'
    if (initializedFor === targetKey) return
    setInitializedFor(targetKey)
    setAdvancedOpen(
      Boolean(
        isEdit &&
        route &&
        (route.displayIcon ||
          route.contextLength ||
          route.modelMapping ||
          (route.routingStrategy && route.routingStrategy !== 'weighted'))
      )
    )
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
      let channelBatchHadErrors = false
      if (isEdit && route) {
        await updateMutation.mutateAsync({ id: route.id, payload })
        routeId = route.id
      } else {
        const result = await createMutation.mutateAsync(payload)
        routeId = resolveCreatedRouteId(result)
        if (!routeId) {
          toast.error(t('tokenRoutes.toast.createFailed'))
          return
        }
      }
      if (routeId && drafts.length > 0) {
        const batchResult = await batchAddChannelsMutation.mutateAsync({
          routeId,
          channels: drafts,
        })
        const channelErrors = batchResult.errors ?? []
        channelBatchHadErrors = channelErrors.length > 0
        if (channelBatchHadErrors) {
          toast.warning(
            t('tokenRoutes.toast.channelAddPartial', {
              created: batchResult.created ?? 0,
              failed: channelErrors.length,
              error: channelErrors[0],
            })
          )
        }
      }
      if (!isEdit && !channelBatchHadErrors) {
        showRouteCompletionToast(routeId, chainContext)
      }
      form.reset()
      onOpenChange(false)
    } catch {
      // Mutation failures (non-2xx and business errors) are already toasted
      // by the http-client response interceptor; swallowing here keeps the
      // sheet open with the user's edits intact so they can retry.
    }
  }

  const onInvalid: SubmitErrorHandler<RouteFormValues> = (errors) => {
    if (
      errors.displayIcon ||
      errors.contextLength ||
      errors.routingStrategy ||
      errors.modelMapping
    ) {
      setAdvancedOpen(true)
    }
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
        className='flex w-full flex-col gap-0 sm:max-w-xl'
        showMobileCloseBar={false}
      >
        <SheetHeader className='shrink-0 border-b px-5 py-5 sm:px-6'>
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
            className='min-h-0 flex-1 space-y-6 overflow-y-auto px-5 py-5 sm:px-6'
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
            {isEdit && route && route.id > 0 && (
              <RouteChannelEditor routeId={route.id} />
            )}
            {routeMode === 'pattern' ? (
              <PatternModeFields
                form={form}
                patternError={patternError}
                accountOptions={accountOptions}
                isEdit={isEdit}
              />
            ) : (
              <GroupModeFields form={form} availableRoutes={availableRoutes} />
            )}
            <Collapsible
              open={advancedOpen}
              onOpenChange={setAdvancedOpen}
              className='border-t pt-4'
            >
              <CollapsibleTrigger className='group focus-visible:ring-ring flex w-full items-center justify-between gap-3 rounded-md py-1 text-left outline-none focus-visible:ring-2'>
                <span className='min-w-0 space-y-1'>
                  <span className='block text-sm font-semibold'>
                    {t('tokenRoutes.form.advancedTitle')}
                  </span>
                  <span className='text-muted-foreground block text-xs leading-5'>
                    {t('tokenRoutes.form.advancedHint')}
                  </span>
                </span>
                <ChevronDown
                  className='text-muted-foreground size-4 shrink-0 transition-transform group-aria-expanded:rotate-180 motion-reduce:transition-none'
                  aria-hidden='true'
                />
              </CollapsibleTrigger>
              <CollapsibleContent keepMounted>
                <RouteAdvancedFields form={form} />
              </CollapsibleContent>
            </Collapsible>
          </form>
        </Form>
        <SheetFooter className='bg-background shrink-0 flex-row justify-end border-t px-5 py-4 sm:px-6'>
          <Button
            variant='outline'
            className='flex-1 sm:flex-none'
            onClick={() => handleOpenChange(false)}
            disabled={isSubmitting}
          >
            {t('common.cancel')}
          </Button>
          <Button
            type='submit'
            form='route-form'
            className='flex-1 sm:flex-none'
            disabled={isSubmitting || !isInitialized}
          >
            {isSubmitting && <Spinner />}
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
  isEdit,
}: {
  form: ReturnType<typeof useForm<RouteFormValues>>
  patternError: string | null
  accountOptions: RouteAccountOption[]
  isEdit: boolean
}) {
  const { t } = useTranslation()
  const bulkLabelId = useId()
  const [accountSearch, setAccountSearch] = useState('')
  const visibleAccounts = useMemo(() => {
    const query = accountSearch.trim().toLocaleLowerCase()
    return query
      ? accountOptions.filter((account) =>
          account.label.toLocaleLowerCase().includes(query)
        )
      : accountOptions
  }, [accountOptions, accountSearch])
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
      <FormField
        control={form.control}
        name='channelDrafts'
        render={({ field }) => {
          const selected = field.value ?? []
          const selectedIds = new Set(selected.map((draft) => draft.accountId))
          const accountIds = visibleAccounts.map((account) => account.id)
          const selectedCount = accountIds.filter((id) =>
            selectedIds.has(id)
          ).length
          const allSelected =
            accountIds.length > 0 && selectedCount === accountIds.length
          const someSelected = selectedCount > 0 && !allSelected
          const toggleAccounts = (ids: number[], checked: boolean) => {
            field.onChange(setChannelDraftSelection(selected, ids, checked))
          }
          let selectKey = allSelected
            ? 'tokenRoutes.formPattern.channelsDeselectAll'
            : 'tokenRoutes.formPattern.channelsSelectAll'
          if (accountSearch.trim()) {
            selectKey = allSelected
              ? 'tokenRoutes.formPattern.channelsDeselectFiltered'
              : 'tokenRoutes.formPattern.channelsSelectFiltered'
          }
          const bulkLabel = t(selectKey)
          const totalSelected = accountOptions.filter((account) =>
            selectedIds.has(account.id)
          ).length
          return (
            <FormItem>
              <FormLabel>{t('tokenRoutes.formPattern.channels')}</FormLabel>
              <FormDescription>
                {t('tokenRoutes.formPattern.channelsHint')}
              </FormDescription>
              {accountOptions.length > 0 ? (
                <div className='overflow-hidden rounded-lg border'>
                  <div className='relative border-b p-2'>
                    <Search
                      className='text-muted-foreground pointer-events-none absolute top-1/2 left-5 size-4 -translate-y-1/2'
                      aria-hidden='true'
                    />
                    <Input
                      type='search'
                      aria-label={t('tokenRoutes.formPattern.searchAccounts')}
                      placeholder={t('tokenRoutes.formPattern.searchAccounts')}
                      value={accountSearch}
                      onChange={(event) => setAccountSearch(event.target.value)}
                      className='border-0 bg-transparent pl-9 shadow-none'
                    />
                  </div>
                  <div className='bg-muted/40 flex flex-wrap items-center justify-between gap-2 border-b px-4 py-2'>
                    <label className='flex items-center gap-2 text-sm'>
                      <FormControl>
                        <Checkbox
                          ref={field.ref}
                          checked={allSelected}
                          disabled={accountIds.length === 0}
                          indeterminate={someSelected}
                          onCheckedChange={(checked) =>
                            toggleAccounts(accountIds, checked)
                          }
                          onBlur={field.onBlur}
                          aria-labelledby={bulkLabelId}
                        />
                      </FormControl>
                      <span id={bulkLabelId}>{bulkLabel}</span>
                    </label>
                    <span
                      role='status'
                      className='text-muted-foreground text-xs tabular-nums'
                    >
                      {t('tokenRoutes.formPattern.channelsSelectedCount', {
                        selected: totalSelected,
                        total: accountOptions.length,
                      })}
                    </span>
                  </div>
                  <div className='max-h-56 space-y-1 overflow-y-auto p-2'>
                    {visibleAccounts.length === 0 && (
                      <p
                        className='text-muted-foreground px-3 py-5 text-center text-sm'
                        role='status'
                      >
                        {t('tokenRoutes.formPattern.noMatchingAccounts')}
                      </p>
                    )}
                    {visibleAccounts.map((account) => (
                      <label
                        key={account.id}
                        className='hover:bg-muted flex min-h-9 items-center gap-3 rounded-md px-2.5 py-2'
                      >
                        <Checkbox
                          checked={selectedIds.has(account.id)}
                          onCheckedChange={(checked) =>
                            toggleAccounts([account.id], checked)
                          }
                        />
                        <span className='truncate text-sm'>
                          {account.label}
                        </span>
                      </label>
                    ))}
                  </div>
                </div>
              ) : (
                <div className='flex flex-col gap-2 rounded-lg border border-dashed p-3'>
                  <p className='text-muted-foreground text-sm'>
                    {t(
                      isEdit
                        ? 'tokenRoutes.formPattern.channelsEmptyHintEdit'
                        : 'tokenRoutes.formPattern.channelsEmptyHint'
                    )}
                  </p>
                </div>
              )}
              <FormMessage />
            </FormItem>
          )
        }}
      />
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
