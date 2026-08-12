// metapi-go/features/settings/hooks — unified settings form hook.
//
// Mirrors the newapi `useSettingsForm` interaction: a server baseline that is
// updated only when the server payload actually changes, dirty-only diffing
// on submit, and a reset that restores the last server state. Sections opt in
// by passing a `serverValues` snapshot derived from GET /api/settings/runtime.

import { zodResolver } from '@hookform/resolvers/zod'
import { useCallback, useEffect, useRef } from 'react'
import {
  useForm,
  type DefaultValues,
  type FieldValues,
  type UseFormReturn,
} from 'react-hook-form'
import type { z } from 'zod'

export type UseSettingsFormOptions<TValues extends FieldValues> = {
  schema: z.ZodType<TValues, any, any>
  defaultValues: DefaultValues<TValues>
  /** Server-derived form values; the form resets (and baseline refreshes) only when this changes. */
  serverValues?: TValues | null
}

export type SettingsFormController<TValues extends FieldValues> = {
  form: UseFormReturn<TValues, any, TValues>
  /** Last server snapshot used for dirty diffing on submit. */
  baseline: TValues | null
  /** Force-reset form + baseline to a server snapshot. */
  syncFromServer: (values: TValues) => void
}

export function useSettingsForm<TValues extends FieldValues>({
  schema,
  defaultValues,
  serverValues,
}: UseSettingsFormOptions<TValues>): SettingsFormController<TValues> {
  const form = useForm<TValues>({
    resolver: zodResolver(schema) as never,
    defaultValues,
  })
  const baselineRef = useRef<TValues | null>(null)
  const lastSyncKeyRef = useRef<string>('')

  const syncFromServer = useCallback(
    (values: TValues) => {
      baselineRef.current = structuredClone(values)
      form.reset(values)
    },
    [form]
  )

  useEffect(() => {
    if (!serverValues) {
      return
    }
    const key = JSON.stringify(serverValues)
    // Skip refetches that return the same server snapshot — the user may be
    // mid-edit and we must not clobber their input.
    if (key === lastSyncKeyRef.current) {
      return
    }
    lastSyncKeyRef.current = key
    syncFromServer(serverValues)
  }, [serverValues, form, syncFromServer])

  return {
    form,
    baseline: baselineRef.current,
    syncFromServer,
  }
}
