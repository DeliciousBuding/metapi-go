import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
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
import { Textarea } from '@/components/ui/textarea'
import { api } from '@/lib/api'
import { toast } from '@/lib/toast'

import { upstreamKeys } from '../lib/upstream-config'
import { modelsCreateSchema, modelNames } from '../lib/upstream-model-schema'

export { UpstreamGrantForm } from './upstream-grant-form'
export { UpstreamModelEditForm } from './upstream-model-edit-form'

export function UpstreamModelCreateForm(props: {
  channelId: number
  onCreated: () => void
  onDirtyChange: (key: string, dirty: boolean) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const form = useForm<z.infer<typeof modelsCreateSchema>>({
    resolver: zodResolver(modelsCreateSchema),
    defaultValues: { names: '' },
  })
  const dirty = form.formState.isDirty
  const onDirtyChange = props.onDirtyChange
  useEffect(() => {
    onDirtyChange('models-new', dirty)
    return () => onDirtyChange('models-new', false)
  }, [dirty, onDirtyChange])
  async function save(values: z.output<typeof modelsCreateSchema>) {
    await api.createUpstreamModels(props.channelId, {
      names: modelNames(values.names),
    })
    form.reset({ names: '' })
    await client.invalidateQueries({ queryKey: upstreamKeys.all })
    props.onCreated()
    toast.success(t('channels.upstream.saved'))
  }
  return (
    <Form {...form}>
      <form
        className='space-y-4 rounded-xl border p-4'
        onSubmit={form.handleSubmit((values) => save(values).catch(() => {}))}
      >
        <FormField
          control={form.control}
          name='names'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('channels.catalog.modelNames')}</FormLabel>
              <FormControl>
                <Textarea
                  {...field}
                  rows={4}
                  disabled={form.formState.isSubmitting}
                  placeholder={'qwen3-coder-plus\ndeepseek-chat'}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <div className='flex justify-end'>
          <Button
            type='submit'
            size='sm'
            disabled={form.formState.isSubmitting}
          >
            {t('channels.catalog.addModels')}
          </Button>
        </div>
      </form>
    </Form>
  )
}
