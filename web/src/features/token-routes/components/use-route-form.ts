import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useMemo, useState } from 'react'
import { useForm, type SubmitErrorHandler } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
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
  transformFormToPayload,
  transformRouteToFormValues,
  type RouteFormValues,
} from '../lib/routes-schema'
import type { RouteMode, RouteSummaryRow } from '../types'
import { showRouteCompletionToast } from './route-completion-toast'

export type RouteFormOptions = {
  open: boolean
  onOpenChange: (open: boolean) => void
  mode: 'create' | 'edit'
  route?: RouteSummaryRow | null
  chainContext?: { accountId?: number; tokenId?: number; siteId?: number }
}

export function useRouteForm({
  open,
  onOpenChange,
  mode,
  route,
  chainContext,
}: RouteFormOptions) {
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
        channelDrafts: buildChannelDraftSeed(
          chainContext?.accountId,
          chainContext?.tokenId
        ),
      })
    }
  }, [open, isEdit, route, initializedFor, chainContext, form])

  const onSubmit = async (values: RouteFormValues) => {
    const payload = transformFormToPayload(values)
    const drafts = (
      values.routeMode === 'pattern' ? (values.channelDrafts ?? []) : []
    ).filter((draft) => draft.accountId > 0)
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
  return {
    isEdit,
    form,
    handleOpenChange,
    guard,
    routeMode,
    advancedOpen,
    setAdvancedOpen,
    isInitialized,
    onSubmit,
    onInvalid,
    isSubmitting,
  }
}
