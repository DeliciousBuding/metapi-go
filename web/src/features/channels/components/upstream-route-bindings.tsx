import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ArrowDown, ArrowUp, Pencil, RotateCcw, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { ModelPill } from '@/components/common/model-pill'
import { useUpstreamDeletion } from '@/components/common/upstream-deletion'
import { Badge } from '@/components/ui/badge'
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
import { Switch } from '@/components/ui/switch'
import { toBcp47 } from '@/i18n/languages'
import { api } from '@/lib/api'
import type {
  ImportedMember,
  ImportedUpstreamInventory,
} from '@/lib/api/imported-upstreams'
import { formatDateTime } from '@/lib/format'
import { toast } from '@/lib/toast'
import { useUndoableDelete } from '@/lib/undoable-delete'

import {
  memberProtocols,
  upstreamKeys,
  upstreamProtocols,
} from '../lib/upstream-config'
import { UpstreamGroupManager } from './upstream-group-manager'

const memberSchema = z.object({
  priority: z.number().int().min(-2147483648).max(2147483647),
  weight: z.number().int().min(1).max(2147483647),
  protocolOrder: z.array(z.number()),
})
type MemberValues = z.infer<typeof memberSchema>

function MemberForm(props: {
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
          render={({ field }) => {
            const inherited = !field.value.length
            const allowed = upstreamProtocols.filter(
              (protocol) => props.member.protocols & protocol.bit
            )
            function move(index: number, offset: number) {
              const next = [...field.value]
              ;[next[index], next[index + offset]] = [
                next[index + offset],
                next[index],
              ]
              field.onChange(next)
            }
            return (
              <FormItem>
                <FormLabel>{t('channels.upstream.protocolOrder')}</FormLabel>
                <FormControl>
                  <div
                    role='group'
                    aria-label={t('channels.upstream.protocolOrder')}
                    tabIndex={-1}
                    ref={field.ref}
                    className='space-y-3'
                  >
                    <label className='flex items-center justify-between gap-3 text-sm'>
                      <span>{t('channels.upstream.inheritProtocols')}</span>
                      <Switch
                        checked={inherited}
                        disabled={pending}
                        onCheckedChange={(checked) =>
                          field.onChange(
                            checked
                              ? []
                              : allowed.map((protocol) => protocol.bit)
                          )
                        }
                      />
                    </label>
                    {!inherited && (
                      <>
                        <div className='flex flex-wrap gap-2'>
                          {allowed.map((protocol) => (
                            <label
                              key={protocol.bit}
                              className='flex items-center gap-2 rounded-full border px-3 py-1.5 text-xs'
                            >
                              <Checkbox
                                disabled={
                                  pending ||
                                  (field.value.length === 1 &&
                                    field.value.includes(protocol.bit))
                                }
                                checked={field.value.includes(protocol.bit)}
                                onCheckedChange={(checked) =>
                                  field.onChange(
                                    checked
                                      ? [...field.value, protocol.bit]
                                      : field.value.filter(
                                          (bit) => bit !== protocol.bit
                                        )
                                  )
                                }
                              />
                              {protocol.name}
                            </label>
                          ))}
                        </div>
                        <ol className='divide-y rounded-lg border'>
                          {field.value.map((bit, index) => (
                            <li
                              key={bit}
                              className='flex items-center gap-3 px-3 py-1.5'
                            >
                              <span className='text-muted-foreground text-xs tabular-nums'>
                                {index + 1}
                              </span>
                              <span className='flex-1 text-sm'>
                                {
                                  upstreamProtocols.find(
                                    (protocol) => protocol.bit === bit
                                  )?.name
                                }
                              </span>
                              <Button
                                type='button'
                                variant='ghost'
                                size='icon-sm'
                                disabled={pending || index === 0}
                                aria-label={t('channels.upstream.moveUp', {
                                  name: upstreamProtocols.find(
                                    (protocol) => protocol.bit === bit
                                  )?.name,
                                })}
                                onClick={() => move(index, -1)}
                              >
                                <ArrowUp className='size-3.5' />
                              </Button>
                              <Button
                                type='button'
                                variant='ghost'
                                size='icon-sm'
                                disabled={
                                  pending || index === field.value.length - 1
                                }
                                aria-label={t('channels.upstream.moveDown', {
                                  name: upstreamProtocols.find(
                                    (protocol) => protocol.bit === bit
                                  )?.name,
                                })}
                                onClick={() => move(index, 1)}
                              >
                                <ArrowDown className='size-3.5' />
                              </Button>
                            </li>
                          ))}
                        </ol>
                      </>
                    )}
                  </div>
                </FormControl>
                <FormMessage />
              </FormItem>
            )
          }}
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

export function UpstreamRouteBindings(props: {
  channelId?: number
  active?: boolean
  beforeDelete?: (action: () => void) => void
  members: ImportedMember[]
  onDirtyChange: (key: string, dirty: boolean) => void
}) {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const [expanded, setExpanded] = useState<number[]>([])
  const [clearing, setClearing] = useState<number | null>(null)
  const { requestDeletion, dialog, isPending } = useUpstreamDeletion()
  const undoableDelete = useUndoableDelete()
  function deleteMember(member: ImportedMember) {
    return requestDeletion(
      {
        kind: 'member',
        id: member.id,
        name: `${member.modelName} · ${member.credentialName}`,
      },
      (_preview, commit) =>
        undoableDelete<ImportedUpstreamInventory, ImportedMember>({
          item: member,
          queryKey: upstreamKeys.all,
          removeFromCache: (data, item) => ({
            ...data,
            members: data.members.filter((entry) => entry.id !== item.id),
          }),
          deleteFn: commit,
          title: t('channels.group.memberDeleted'),
          undoLabel: t('common.undo'),
          errorTitle: t('channels.group.deleteError'),
        }),
      () => props.onDirtyChange(`member-${member.id}`, false)
    )
  }
  async function clearCooldown(id: number) {
    setClearing(id)
    try {
      const result = await api.clearImportedMemberCooldown(id)
      if (result.success) {
        await client.invalidateQueries({ queryKey: upstreamKeys.all })
        await client.invalidateQueries({ queryKey: ['routes'] })
        toast.success(t('channels.upstream.cooldownCleared'))
      }
    } finally {
      setClearing(null)
    }
  }
  return (
    <div className='space-y-3'>
      {props.channelId !== undefined && (
        <UpstreamGroupManager
          channelId={props.channelId}
          active={props.active ?? false}
          onDirtyChange={props.onDirtyChange}
          beforeDelete={props.beforeDelete}
        />
      )}
      {props.channelId !== undefined && (
        <h3 className='pt-3 text-sm font-semibold'>
          {t('channels.group.members')}
        </h3>
      )}
      {props.members.map((member) => {
        const cooling =
          !!member.cooldownUntil &&
          Date.parse(member.cooldownUntil) > Date.now()
        let status = 'channels.upstream.routable'
        let tone: 'success' | 'warning' | 'secondary' = 'success'
        if (!member.effectiveEnabled) {
          status = 'channels.upstream.inactive'
          tone = 'secondary'
        } else if (cooling) {
          status = 'channels.imported.coolingDown'
          tone = 'warning'
        }
        return (
          <article
            key={member.id}
            className='overflow-hidden rounded-xl border'
          >
            <div className='space-y-3 p-4'>
              <div className='flex flex-wrap items-start justify-between gap-3'>
                <div className='min-w-0 flex-1 space-y-2'>
                  <Link
                    to='/token-routes'
                    search={{ routeId: member.routeId }}
                    className='hover:text-primary text-sm font-semibold hover:underline'
                  >
                    {member.groupName}
                  </Link>
                  <div>
                    <ModelPill model={member.modelName} />
                  </div>
                </div>
                <div className='flex items-center gap-1'>
                  <Badge variant={tone}>{t(status)}</Badge>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon-sm'
                    aria-label={t('channels.upstream.editRoute', {
                      name: member.groupName,
                    })}
                    aria-expanded={expanded.includes(member.id)}
                    onClick={() =>
                      setExpanded((ids) =>
                        ids.includes(member.id)
                          ? ids.filter((id) => id !== member.id)
                          : [...ids, member.id]
                      )
                    }
                  >
                    <Pencil className='size-4' />
                  </Button>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon-sm'
                    disabled={isPending}
                    aria-label={t('channels.group.deleteMember', {
                      name: member.modelName,
                    })}
                    onClick={() => {
                      const action = () => {
                        void deleteMember(member)
                      }
                      if (props.beforeDelete) props.beforeDelete(action)
                      else action()
                    }}
                  >
                    <Trash2 className='size-4' />
                  </Button>
                </div>
              </div>
              <div className='text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs'>
                <span>{member.credentialName}</span>
                <span>{memberProtocols(member)}</span>
                <span>
                  {t('channels.upstream.attempts', {
                    success: member.successCount ?? 0,
                    fail: member.failCount ?? 0,
                  })}
                </span>
              </div>
              {!member.effectiveEnabled && (
                <div className='text-muted-foreground flex flex-wrap gap-2 text-xs'>
                  {(
                    [
                      'channelEnabled',
                      'credentialEnabled',
                      'modelEnabled',
                      'routeEnabled',
                      'groupEnabled',
                      'grantEnabled',
                      'selectedByGroup',
                    ] as const
                  )
                    .filter((key) => member[key] === false)
                    .map((key) => (
                      <span key={key}>
                        {t(`channels.upstream.reasons.${key}`)}
                      </span>
                    ))}
                </div>
              )}
              {cooling && (
                <div className='bg-muted/50 flex flex-wrap items-center justify-between gap-3 rounded-lg px-3 py-2'>
                  <div className='space-y-1 text-xs'>
                    <p>
                      {t('channels.upstream.cooldownUntil', {
                        value: formatDateTime(
                          member.cooldownUntil,
                          toBcp47(i18n.language)
                        ),
                      })}
                    </p>
                    {member.cooldownReasonCode && (
                      <p className='text-muted-foreground'>
                        {member.cooldownReasonCode}
                      </p>
                    )}
                    <p className='text-muted-foreground'>
                      {t('channels.upstream.sharedCooldown')}
                    </p>
                  </div>
                  <Button
                    type='button'
                    size='sm'
                    variant='outline'
                    disabled={clearing !== null}
                    onClick={() =>
                      void clearCooldown(member.id).catch(() => {})
                    }
                  >
                    <RotateCcw className='size-3.5' />
                    {t('channels.upstream.clearCooldown')}
                  </Button>
                </div>
              )}
            </div>
            <div hidden={!expanded.includes(member.id)}>
              <MemberForm member={member} onDirtyChange={props.onDirtyChange} />
            </div>
          </article>
        )
      })}
      {!props.members.length && (
        <p className='text-muted-foreground py-8 text-center'>
          {t('channels.upstream.noRoutes')}
        </p>
      )}
      {dialog}
    </div>
  )
}
