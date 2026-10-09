import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Form,
  FormControl,
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
import { Switch } from '@/components/ui/switch'
import { api } from '@/lib/api'
import type { UpstreamGrant, UpstreamGroup } from '@/lib/api/upstream-catalog'
import { toast } from '@/lib/toast'

import { upstreamKeys } from '../lib/upstream-config'
import {
  upstreamGroupSchema,
  type UpstreamGroupValues,
} from '../lib/upstream-group-schema'

export type GroupGrantOption = UpstreamGrant & { label: string }

function groupValues(group?: UpstreamGroup): UpstreamGroupValues {
  return {
    name: group?.name ?? '',
    modelPattern: group?.modelPattern ?? '',
    grantIds: group?.members.map((member) => member.grantId) ?? [],
    mode: group?.mode ?? 'failover',
    enabled: group?.enabled ?? false,
    activeId: group?.activeMemberId ?? 0,
  }
}

export function UpstreamGroupForm(props: {
  group?: UpstreamGroup
  grants: GroupGrantOption[]
  onDirtyChange: (key: string, dirty: boolean) => void
  onCreated?: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const group = props.group
  const dirtyKey = group ? `group-${group.id}` : 'group-create'
  const onDirtyChange = props.onDirtyChange
  const form = useForm<UpstreamGroupValues>({
    resolver: zodResolver(upstreamGroupSchema(!group)),
    defaultValues: groupValues(group),
  })
  const dirty = form.formState.isDirty
  const dirtyFields = form.formState.dirtyFields
  useEffect(() => {
    onDirtyChange(dirtyKey, dirty)
  }, [dirtyKey, dirty, onDirtyChange])
  useEffect(
    () => () => onDirtyChange(dirtyKey, false),
    [dirtyKey, onDirtyChange]
  )
  useEffect(() => {
    if (group && !form.formState.isDirty) {
      form.reset(groupValues(group))
    }
  }, [group, form])
  const grantIds = form.watch('grantIds')
  const activeOptions = group
    ? group.members.map((member) => ({
        id: member.id,
        label: `${member.modelName} · ${member.credentialName} · ${t('channels.group.channel', { id: member.channelId })}`,
      }))
    : props.grants
        .filter((grant) => grantIds.includes(grant.id))
        .map((grant) => ({ id: grant.id, label: grant.label }))
  const mutation = useMutation({
    mutationFn: (values: UpstreamGroupValues) =>
      group
        ? api.updateUpstreamGroup(group.id, {
            ...(dirtyFields.name ? { name: values.name } : {}),
            ...(dirtyFields.mode ? { mode: values.mode } : {}),
            ...(dirtyFields.enabled ? { enabled: values.enabled } : {}),
            ...(dirtyFields.activeId
              ? { activeMemberId: values.activeId }
              : {}),
          })
        : api.createUpstreamGroup({
            name: values.name,
            mode: values.mode,
            enabled: values.enabled,
            route: { modelPattern: values.modelPattern },
            members: values.grantIds.map((grantId) => ({ grantId })),
            ...(values.activeId ? { activeGrantId: values.activeId } : {}),
          }),
    onSuccess: async () => {
      await client.invalidateQueries({ queryKey: upstreamKeys.all })
      await client.invalidateQueries({ queryKey: ['routes'] })
    },
  })
  async function save(values: UpstreamGroupValues) {
    if (
      !group &&
      values.grantIds.some(
        (id) => !props.grants.some((grant) => grant.id === id)
      )
    ) {
      form.setError('grantIds', { message: 'channels.group.grantsChanged' })
      return
    }
    if (
      values.activeId &&
      (!group || dirtyFields.activeId) &&
      !activeOptions.some((option) => option.id === values.activeId)
    ) {
      form.setError('activeId', { message: 'channels.group.activeRequired' })
      return
    }
    try {
      const saved = await mutation.mutateAsync(values)
      form.reset(groupValues(group ? saved : undefined))
      toast.success(t('channels.upstream.saved'))
      if (!group) props.onCreated?.()
    } catch (error) {
      form.setError('root', {
        message:
          error instanceof Error
            ? error.message
            : t('channels.group.saveError'),
      })
    }
  }
  const pending = mutation.isPending
  return (
    <Form {...form}>
      <form
        className='space-y-4 border-t p-4'
        aria-label={t(
          group ? 'channels.group.editForm' : 'channels.group.create'
        )}
        onSubmit={form.handleSubmit(save)}
      >
        <div className='grid gap-4 sm:grid-cols-2'>
          <FormField
            control={form.control}
            name='name'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('channels.group.name')}</FormLabel>
                <FormControl>
                  <Input {...field} disabled={pending} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          {!group && (
            <FormField
              control={form.control}
              name='modelPattern'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('channels.group.publicModel')}</FormLabel>
                  <FormControl>
                    <Input {...field} disabled={pending} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}
        </div>
        {!group && (
          <FormField
            control={form.control}
            name='grantIds'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('channels.group.grants')}</FormLabel>
                <FormControl>
                  <div
                    role='group'
                    aria-label={t('channels.group.grants')}
                    tabIndex={-1}
                    ref={field.ref}
                    className='max-h-56 space-y-2 overflow-y-auto rounded-lg border p-3'
                  >
                    {props.grants.map((grant) => (
                      <label
                        key={grant.id}
                        className='flex items-start gap-2 text-sm'
                      >
                        <Checkbox
                          disabled={pending}
                          checked={field.value.includes(grant.id)}
                          onCheckedChange={(checked) =>
                            field.onChange(
                              checked
                                ? [...field.value, grant.id]
                                : field.value.filter((id) => id !== grant.id)
                            )
                          }
                        />
                        <span className='min-w-0 break-words'>
                          {grant.label}
                        </span>
                      </label>
                    ))}
                    {!props.grants.length && (
                      <p className='text-muted-foreground text-sm'>
                        {t('channels.group.noGrants')}
                      </p>
                    )}
                  </div>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        )}
        <div className='grid gap-4 sm:grid-cols-2'>
          <FormField
            control={form.control}
            name='mode'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('channels.group.mode')}</FormLabel>
                <Select
                  value={field.value}
                  items={(['failover', 'manual'] as const).map((mode) => ({
                    value: mode,
                    label: t(`channels.group.${mode}`),
                  }))}
                  onValueChange={field.onChange}
                  disabled={pending}
                >
                  <FormControl>
                    <SelectTrigger className='w-full' ref={field.ref}>
                      <SelectValue />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    {(['failover', 'manual'] as const).map((mode) => (
                      <SelectItem key={mode} value={mode}>
                        {t(`channels.group.${mode}`)}
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
            name='enabled'
            render={({ field }) => (
              <FormItem className='flex items-center justify-between gap-3 sm:pt-6'>
                <FormLabel>{t('channels.imported.enabled')}</FormLabel>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                    disabled={pending}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </div>
        {form.watch('mode') === 'manual' && (
          <FormField
            control={form.control}
            name='activeId'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('channels.group.activeMember')}</FormLabel>
                <Select
                  value={String(field.value)}
                  items={[
                    { value: '0', label: t('channels.group.chooseActive') },
                    ...activeOptions.map((option) => ({
                      value: String(option.id),
                      label: option.label,
                    })),
                  ]}
                  onValueChange={(value) => field.onChange(Number(value))}
                  disabled={pending}
                >
                  <FormControl>
                    <SelectTrigger className='w-full min-w-0' ref={field.ref}>
                      <SelectValue className='truncate' />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    <SelectItem value='0'>
                      {t('channels.group.chooseActive')}
                    </SelectItem>
                    {activeOptions.map((option) => (
                      <SelectItem key={option.id} value={String(option.id)}>
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
            )}
          />
        )}
        {form.formState.errors.root && (
          <p role='alert' className='text-destructive text-sm'>
            {form.formState.errors.root.message}
          </p>
        )}
        <div className='flex justify-end'>
          <Button size='sm' type='submit' disabled={!dirty || pending}>
            {t(group ? 'common.save' : 'channels.group.create')}
          </Button>
        </div>
      </form>
    </Form>
  )
}
