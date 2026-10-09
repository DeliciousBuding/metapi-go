import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { useUpstreamDeletion } from '@/components/common/upstream-deletion'
import { Badge } from '@/components/ui/badge'
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
import { useUndoableDelete } from '@/lib/undoable-delete'

import { upstreamKeys } from '../lib/upstream-config'
import {
  upstreamGroupBindingSchema,
  type UpstreamGroupBindingValues,
} from '../lib/upstream-group-schema'
import { UpstreamGroupForm, type GroupGrantOption } from './upstream-group-form'

function GroupBindingForm(props: {
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

export function UpstreamGroupManager(props: {
  channelId: number
  active: boolean
  onDirtyChange: (key: string, dirty: boolean) => void
  beforeDelete?: (action: () => void) => void
}) {
  const { t } = useTranslation()
  const [creating, setCreating] = useState(false)
  const [binding, setBinding] = useState(false)
  const [expanded, setExpanded] = useState<number[]>([])
  const modelsQuery = useQuery({
    queryKey: upstreamKeys.models(props.channelId),
    queryFn: () => api.getUpstreamModels(props.channelId),
    enabled: props.active,
  })
  const credentialsQuery = useQuery({
    queryKey: upstreamKeys.credentials(props.channelId),
    queryFn: () => api.getImportedCredentials(props.channelId),
    enabled: props.active,
  })
  const groupsQuery = useQuery({
    queryKey: upstreamKeys.groups,
    queryFn: api.getUpstreamGroups,
    enabled: props.active,
  })
  const grants: GroupGrantOption[] = (modelsQuery.data?.items ?? []).flatMap(
    (model) =>
      model.grants.map((grant) => ({
        ...grant,
        label: `${model.name} · ${credentialsQuery.data?.items.find((item) => item.id === grant.credentialId)?.name ?? grant.credentialName}${model.enabled && grant.enabled && credentialsQuery.data?.items.find((item) => item.id === grant.credentialId)?.enabled ? '' : ` · ${t('channels.group.inactive')}`}`,
      }))
  )
  const groups = groupsQuery.data?.items ?? []
  const relatedGroups = groups.filter((group) =>
    group.members.some((member) => member.channelId === props.channelId)
  )
  const { requestDeletion, dialog, isPending } = useUpstreamDeletion()
  const undoableDelete = useUndoableDelete()
  function removeGroup(group: UpstreamGroup) {
    return requestDeletion(
      { kind: 'group', id: group.id, name: group.name },
      (_preview, commit) =>
        undoableDelete<{ items: UpstreamGroup[] }, UpstreamGroup>({
          item: group,
          queryKey: upstreamKeys.groups,
          removeFromCache: (data, item) => ({
            ...data,
            items: data.items.filter((entry) => entry.id !== item.id),
          }),
          deleteFn: commit,
          title: t('channels.group.deleted'),
          undoLabel: t('common.undo'),
          errorTitle: t('channels.group.deleteError'),
        }),
      () => {
        props.onDirtyChange(`group-${group.id}`, false)
        group.members.forEach((member) =>
          props.onDirtyChange(`member-${member.id}`, false)
        )
      }
    )
  }
  const queries = {
    models: modelsQuery,
    credentials: credentialsQuery,
    groups: groupsQuery,
  }
  const loading = Object.values(queries).some((query) => query.isLoading)
  const failed = Object.values(queries).some((query) => query.isError)
  return (
    <section className='space-y-3' aria-label={t('channels.group.title')}>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <h3 className='text-sm font-semibold'>{t('channels.group.title')}</h3>
        <div className='flex flex-wrap gap-2'>
          <Button
            size='sm'
            variant='outline'
            disabled={loading || failed}
            aria-expanded={creating}
            onClick={() => setCreating((value) => !value)}
          >
            <Plus className='size-3.5' />
            {t('channels.group.create')}
          </Button>
          <Button
            size='sm'
            variant='outline'
            disabled={loading || failed || !groups.length}
            aria-expanded={binding}
            onClick={() => setBinding((value) => !value)}
          >
            {t('channels.group.bind')}
          </Button>
        </div>
      </div>
      {Object.entries(queries).map(
        ([key, query]) =>
          query.error && (
            <QueryErrorBanner
              key={key}
              error={query.error}
              messageKey='channels.group.loadError'
              onRetry={() => void query.refetch()}
              isRetrying={query.isFetching}
            />
          )
      )}
      {loading && (
        <p className='text-muted-foreground text-sm'>
          {t('channels.upstream.loading')}
        </p>
      )}
      <div hidden={!creating} className='overflow-hidden rounded-xl border'>
        <UpstreamGroupForm
          grants={grants}
          onDirtyChange={props.onDirtyChange}
          onCreated={() => setCreating(false)}
        />
      </div>
      <div hidden={!binding}>
        <GroupBindingForm
          groups={groups}
          grants={grants}
          onDirtyChange={props.onDirtyChange}
          onSaved={() => setBinding(false)}
        />
      </div>
      {relatedGroups.map((group) => (
        <article key={group.id} className='overflow-hidden rounded-xl border'>
          <div className='flex flex-wrap items-center justify-between gap-3 p-4'>
            <div className='min-w-0 flex-1'>
              <p className='text-sm font-semibold break-words'>{group.name}</p>
              <Link
                to='/token-routes'
                search={{ routeId: group.routeId }}
                className='text-muted-foreground text-xs break-all hover:underline'
              >
                {group.displayName || group.modelPattern}
              </Link>
              <p className='text-muted-foreground mt-1 text-xs'>
                {t('channels.group.memberCount', {
                  count: group.members.length,
                })}
              </p>
            </div>
            <div className='flex items-center gap-1'>
              <Badge variant={group.enabled ? 'success' : 'secondary'}>
                {t(
                  group.enabled
                    ? `channels.group.${group.mode}`
                    : 'channels.group.inactive'
                )}
              </Badge>
              <Button
                size='icon-sm'
                variant='ghost'
                aria-label={t('channels.group.edit', { name: group.name })}
                aria-expanded={expanded.includes(group.id)}
                onClick={() =>
                  setExpanded((ids) =>
                    ids.includes(group.id)
                      ? ids.filter((id) => id !== group.id)
                      : [...ids, group.id]
                  )
                }
              >
                <Pencil className='size-4' />
              </Button>
              <Button
                size='icon-sm'
                variant='ghost'
                disabled={isPending}
                aria-label={t('channels.group.delete', { name: group.name })}
                onClick={() => {
                  const action = () => {
                    void removeGroup(group)
                  }
                  if (props.beforeDelete) props.beforeDelete(action)
                  else action()
                }}
              >
                <Trash2 className='size-4' />
              </Button>
            </div>
          </div>
          <div hidden={!expanded.includes(group.id)}>
            <UpstreamGroupForm
              group={group}
              grants={grants}
              onDirtyChange={props.onDirtyChange}
            />
          </div>
        </article>
      ))}
      {dialog}
    </section>
  )
}
