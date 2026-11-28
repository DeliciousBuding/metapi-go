import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

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
import { api } from '@/lib/api'
import type { ImportedMember } from '@/lib/api/imported-upstreams'
import { toast } from '@/lib/toast'

import { upstreamKeys } from '../lib/upstream-config'
import { MemberProtocolOrder } from './upstream-member-protocol-order'

const memberSchema = z.object({
  priority: z.number().int().min(-2147483648).max(2147483647),
  weight: z.number().int().min(1).max(2147483647),
  protocolOrder: z.array(z.number()),
})
type MemberValues = z.infer<typeof memberSchema>

export function MemberForm(props: {
  member: ImportedMember
  onDirtyChange: (key: string, dirty: boolean) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const form = useForm<MemberValues>({
    resolver: zodResolver(memberSchema),
    defaultValues: {
      priority: props.member.priority,
      weight: props.member.weight,
      protocolOrder: props.member.protocolOrder ?? [],
    },
  })
  const pending = form.formState.isSubmitting
  const dirty = form.formState.isDirty
  const dirtyFields = form.formState.dirtyFields
  const onDirtyChange = props.onDirtyChange
  useEffect(() => {
    onDirtyChange(`member-${props.member.id}`, dirty)
  }, [props.member.id, dirty, onDirtyChange])
  useEffect(
    () => () => onDirtyChange(`member-${props.member.id}`, false),
    [props.member.id, onDirtyChange]
  )
  useEffect(() => {
    form.reset(
      {
        priority: props.member.priority,
        weight: props.member.weight,
        protocolOrder: props.member.protocolOrder ?? [],
      },
      { keepDirtyValues: true }
    )
  }, [props.member, form])
  async function save(values: MemberValues) {
    try {
      const result = await api.updateImportedMember(props.member.id, {
        ...(dirtyFields.priority ? { priority: values.priority } : {}),
        ...(dirtyFields.weight ? { weight: values.weight } : {}),
        ...(dirtyFields.protocolOrder
          ? { protocolOrder: values.protocolOrder }
          : {}),
      })
      if (!result.success) throw new Error(t('channels.group.saveError'))
      await client.invalidateQueries({ queryKey: upstreamKeys.all })
      await client.invalidateQueries({ queryKey: ['routes'] })
      form.reset(form.getValues())
      toast.success(t('channels.upstream.saved'))
    } catch (error) {
      form.setError('root', {
        message:
          error instanceof Error
            ? error.message
            : t('channels.group.saveError'),
      })
    }
  }
  return (
    <Form {...form}>
      <form
        className='space-y-4 border-t p-4'
        onSubmit={form.handleSubmit(save)}
      >
        <div className='grid grid-cols-2 gap-4'>
          {(['priority', 'weight'] as const).map((name) => (
            <FormField
              key={name}
              control={form.control}
              name={name}
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t(`channels.columns.${name}`)}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type='number'
                      step={1}
                      disabled={pending}
                      onChange={(event) =>
                        field.onChange(
                          event.target.value === ''
                            ? Number.NaN
                            : Number(event.target.value)
                        )
                      }
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          ))}
        </div>
        <FormField
          control={form.control}
          name='protocolOrder'
          render={({ field }) => (
            <MemberProtocolOrder
              field={field}
              protocols={props.member.protocols}
              pending={pending}
            />
          )}
        />
        {form.formState.errors.root && (
          <p role='alert' className='text-destructive text-sm'>
            {form.formState.errors.root.message}
          </p>
        )}
        <div className='flex justify-end'>
          <Button type='submit' size='sm' disabled={!dirty || pending}>
            {t('channels.upstream.saveRoute')}
          </Button>
        </div>
      </form>
    </Form>
  )
}
