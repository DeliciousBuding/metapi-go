import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { api } from '@/lib/api'
import type { UpstreamGroup } from '@/lib/api/upstream-catalog'
import { toast } from '@/lib/toast'

import { upstreamKeys } from '../lib/upstream-config'
import {
  upstreamGroupBindingSchema,
  type UpstreamGroupBindingValues,
} from '../lib/upstream-group-schema'
import type { GroupGrantOption } from './upstream-group-form'

export function GroupBindingForm(props: {
  groups: UpstreamGroup[]
  grants: GroupGrantOption[]
  onDirtyChange: (key: string, dirty: boolean) => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const form = useForm<UpstreamGroupBindingValues>({
    resolver: zodResolver(upstreamGroupBindingSchema),
    defaultValues: { groupId: 0, grantId: 0 },
  })
  const dirty = form.formState.isDirty
  const onDirtyChange = props.onDirtyChange
  useEffect(() => {
    onDirtyChange('group-bind', dirty)
  }, [dirty, onDirtyChange])
  useEffect(() => () => onDirtyChange('group-bind', false), [onDirtyChange])
  const group = props.groups.find((item) => item.id === form.watch('groupId'))
  const options = props.grants.filter(
    (grant) => !group?.members.some((member) => member.grantId === grant.id)
  )
  const mutation = useMutation({
    mutationFn: (values: UpstreamGroupBindingValues) =>
      api.createUpstreamMember(values.groupId, { grantId: values.grantId }),
    onSuccess: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: upstreamKeys.all }),
        client.invalidateQueries({ queryKey: ['routes'] }),
      ])
    },
  })
  async function save(values: UpstreamGroupBindingValues) {
    if (!group) {
      form.setError('groupId', { message: 'channels.group.groupRequired' })
      return
    }
    if (!options.some((option) => option.id === values.grantId)) {
      form.setError('grantId', { message: 'channels.group.grantsChanged' })
      return
    }
    try {
      await mutation.mutateAsync(values)
      form.reset({ groupId: 0, grantId: 0 })
      toast.success(t('channels.upstream.saved'))
      props.onSaved()
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
        aria-label={t('channels.group.bind')}
        onSubmit={form.handleSubmit(save)}
        className='space-y-4 rounded-xl border p-4'
      >
        <FormField
          control={form.control}
          name='groupId'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('channels.group.targetGroup')}</FormLabel>
              <Select
                value={String(field.value)}
                items={[
                  { value: '0', label: t('channels.group.chooseGroup') },
                  ...props.groups.map((item) => ({
                    value: String(item.id),
                    label: `${item.name} · ${item.modelPattern}`,
                  })),
                ]}
                onValueChange={(value) => field.onChange(Number(value))}
                disabled={mutation.isPending}
              >
                <FormControl>
                  <SelectTrigger ref={field.ref} className='w-full min-w-0'>
                    <SelectValue className='truncate' />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  <SelectItem value='0'>
                    {t('channels.group.chooseGroup')}
                  </SelectItem>
                  {props.groups.map((item) => (
                    <SelectItem key={item.id} value={String(item.id)}>
                      {item.name} · {item.modelPattern}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='grantId'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('channels.group.grant')}</FormLabel>
              <Select
                value={String(field.value)}
                items={[
                  { value: '0', label: t('channels.group.chooseGrant') },
                  ...options.map((option) => ({
                    value: String(option.id),
                    label: option.label,
                  })),
                ]}
                onValueChange={(value) => field.onChange(Number(value))}
                disabled={mutation.isPending || !group}
              >
                <FormControl>
                  <SelectTrigger ref={field.ref} className='w-full min-w-0'>
                    <SelectValue className='truncate' />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  <SelectItem value='0'>
                    {t('channels.group.chooseGrant')}
                  </SelectItem>
                  {options.map((grant) => (
                    <SelectItem key={grant.id} value={String(grant.id)}>
                      {grant.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FormMessage />
            </FormItem>
          )}
        />
        {group && !options.length && (
          <p className='text-muted-foreground text-sm'>
            {t('channels.group.noUnboundGrants')}
          </p>
        )}
        {form.formState.errors.root && (
          <p role='alert' className='text-destructive text-sm'>
            {form.formState.errors.root.message}
          </p>
        )}
        <div className='flex justify-end'>
          <Button
            type='submit'
            size='sm'
            disabled={!dirty || mutation.isPending}
          >
            {t('channels.group.bind')}
          </Button>
        </div>
      </form>
    </Form>
  )
}
