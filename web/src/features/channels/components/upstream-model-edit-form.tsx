import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import type { z } from 'zod'

import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { api } from '@/lib/api'
import type { UpstreamModel } from '@/lib/api/upstream-catalog'
import { toast } from '@/lib/toast'

import { upstreamKeys } from '../lib/upstream-config'
import { modelSchema } from '../lib/upstream-model-schema'

export function UpstreamModelEditForm(props: {
  model: UpstreamModel
  onDirtyChange: (key: string, dirty: boolean) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const form = useForm<z.infer<typeof modelSchema>>({
    resolver: zodResolver(modelSchema),
    values: { name: props.model.name, enabled: props.model.enabled },
    resetOptions: { keepDirtyValues: true },
  })
  const [saving, setSaving] = useState(false)
  const submitting = useRef(false)
  const pending = saving || form.formState.isSubmitting
  const dirty = form.formState.isDirty
  const onDirtyChange = props.onDirtyChange
  useEffect(() => {
    const key = `model-${props.model.id}`
    onDirtyChange(key, dirty)
    return () => onDirtyChange(key, false)
  }, [dirty, onDirtyChange, props.model.id])
  async function save(values: z.infer<typeof modelSchema>) {
    if (submitting.current) return
    submitting.current = true
    setSaving(true)
    try {
      await api.updateUpstreamModel(props.model.id, {
        ...(form.formState.dirtyFields.name ? { name: values.name } : {}),
        ...(form.formState.dirtyFields.enabled
          ? { enabled: values.enabled }
          : {}),
      })
      form.reset(
        {
          name: form.formState.dirtyFields.name
            ? values.name
            : props.model.name,
          enabled: form.formState.dirtyFields.enabled
            ? values.enabled
            : props.model.enabled,
        },
        { keepDirtyValues: false }
      )
      await client.invalidateQueries({ queryKey: upstreamKeys.all })
      await client.invalidateQueries({ queryKey: ['routes'] })
      toast.success(t('channels.upstream.saved'))
    } finally {
      submitting.current = false
      setSaving(false)
    }
  }
  return (
    <Form {...form}>
      <form
        className='space-y-4'
        onSubmit={form.handleSubmit((values) => save(values).catch(() => {}))}
      >
        <div className='grid items-end gap-3 sm:grid-cols-[1fr_auto]'>
          <FormField
            control={form.control}
            name='name'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('channels.catalog.actualModel')}</FormLabel>
                <FormControl>
                  <Input {...field} disabled={pending} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <FormItem className='flex items-center gap-3 pb-2'>
                <FormLabel>{t('channels.imported.enabled')}</FormLabel>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={pending}
                  />
                </FormControl>
              </FormItem>
            )}
          />
        </div>
        <div className='flex justify-end'>
          <Button type='submit' size='sm' disabled={!dirty || pending}>
            {t('channels.catalog.saveModel')}
          </Button>
        </div>
      </form>
    </Form>
  )
}
