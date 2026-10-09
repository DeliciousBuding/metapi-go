import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { isAxiosError } from 'axios'
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import type { z } from 'zod'

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
import { Textarea } from '@/components/ui/textarea'
import { api } from '@/lib/api'
import type {
  ImportedCredential,
  ImportedMember,
} from '@/lib/api/imported-upstreams'
import type { UpstreamGrant, UpstreamModel } from '@/lib/api/upstream-catalog'
import { toast } from '@/lib/toast'

import { upstreamKeys, upstreamProtocols } from '../lib/upstream-config'
import {
  grantSchema,
  modelSchema,
  modelsCreateSchema,
  modelNames,
} from '../lib/upstream-model-schema'

type DirtyProps = { onDirtyChange: (key: string, dirty: boolean) => void }

export function UpstreamModelCreateForm(
  props: DirtyProps & { channelId: number; onCreated: () => void }
) {
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

export function UpstreamModelEditForm(
  props: DirtyProps & { model: UpstreamModel }
) {
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

export function UpstreamGrantForm(
  props: DirtyProps & {
    modelId: number
    grant?: UpstreamGrant
    credentials: ImportedCredential[]
    availableProtocols: number[]
    members: ImportedMember[]
    onCreated?: () => void
  }
) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const form = useForm<z.infer<typeof grantSchema>>({
    resolver: zodResolver(grantSchema),
    resetOptions: { keepDirtyValues: true },
    values: {
      credentialId: props.grant?.credentialId ?? 0,
      protocols: props.grant
        ? upstreamProtocols
            .filter((p) => p.bit & (props.grant?.protocols ?? 0))
            .map((p) => p.bit)
        : [],
      enabled: props.grant?.enabled ?? true,
    },
  })
  const [saving, setSaving] = useState(false)
  const submitting = useRef(false)
  const pending = saving || form.formState.isSubmitting
  const dirty = form.formState.isDirty
  const onDirtyChange = props.onDirtyChange
  useEffect(() => {
    const key = `grant-${props.grant?.id ?? `new-${props.modelId}`}`
    onDirtyChange(key, dirty)
    return () => onDirtyChange(key, false)
  }, [dirty, onDirtyChange, props.grant?.id, props.modelId])
  async function save(values: z.infer<typeof grantSchema>) {
    if (submitting.current) return
    submitting.current = true
    setSaving(true)
    try {
      form.clearErrors('root')
      try {
        if (props.grant) {
          await api.updateUpstreamGrant(props.grant.id, {
            ...(form.formState.dirtyFields.protocols
              ? { protocols: values.protocols }
              : {}),
            ...(form.formState.dirtyFields.enabled
              ? { enabled: values.enabled }
              : {}),
          })
        } else {
          await api.createUpstreamGrant({ ...values, modelId: props.modelId })
        }
        form.reset(
          props.grant
            ? {
                ...values,
                protocols: form.formState.dirtyFields.protocols
                  ? values.protocols
                  : upstreamProtocols
                      .filter((p) => p.bit & (props.grant?.protocols ?? 0))
                      .map((p) => p.bit),
                enabled: form.formState.dirtyFields.enabled
                  ? values.enabled
                  : props.grant.enabled,
              }
            : { credentialId: 0, protocols: [], enabled: true },
          { keepDirtyValues: false }
        )
        await client.invalidateQueries({ queryKey: upstreamKeys.all })
        await client.invalidateQueries({ queryKey: ['routes'] })
        props.onCreated?.()
        toast.success(t('channels.upstream.saved'))
      } catch (error) {
        if (!props.grant) return // The transport owns create errors.
        const ids: number[] = isAxiosError(error)
          ? (error.response?.data?.conflictingMemberIds ?? [])
          : []
        const routes = props.members
          .filter((member) => ids.includes(member.id))
          .map((member) => member.groupName)
        form.setError('root', {
          message: ids.length
            ? t('channels.catalog.protocolConflict', {
                names: [...new Set(routes)].join(', ') || ids.join(', '),
              })
            : t('channels.catalog.saveError'),
        })
      }
    } finally {
      submitting.current = false
      setSaving(false)
    }
  }
  return (
    <Form {...form}>
      <form
        className='space-y-4 rounded-lg border p-3'
        onSubmit={form.handleSubmit(save)}
      >
        {props.grant ? (
          <p className='text-sm font-medium'>{props.grant.credentialName}</p>
        ) : (
          <FormField
            control={form.control}
            name='credentialId'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('channels.upstream.credentials')}</FormLabel>
                <Select
                  value={field.value ? String(field.value) : ''}
                  onValueChange={(value) => field.onChange(Number(value))}
                  disabled={pending}
                >
                  <FormControl>
                    <SelectTrigger>
                      <SelectValue
                        placeholder={t('channels.catalog.selectCredential')}
                      />
                    </SelectTrigger>
                  </FormControl>
                  <SelectContent>
                    {props.credentials.map((credential) => (
                      <SelectItem
                        key={credential.id}
                        value={String(credential.id)}
                      >
                        {credential.name}
                        {!credential.enabled &&
                          ` · ${t('channels.imported.disabled')}`}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <FormMessage />
              </FormItem>
            )}
          />
        )}
        <FormField
          control={form.control}
          name='protocols'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('channels.catalog.allowedProtocols')}</FormLabel>
              <FormControl>
                <div
                  ref={field.ref}
                  role='group'
                  className='flex flex-wrap gap-2'
                >
                  {upstreamProtocols
                    .filter(
                      (p) =>
                        props.availableProtocols.includes(p.bit) ||
                        field.value.includes(p.bit)
                    )
                    .map((protocol) => (
                      <label
                        key={protocol.bit}
                        className='flex items-center gap-2 rounded-full border px-3 py-1.5 text-xs'
                      >
                        <Checkbox
                          checked={field.value.includes(protocol.bit)}
                          disabled={pending}
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
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <div className='flex items-center justify-between gap-3'>
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <FormItem className='flex items-center gap-3'>
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
          <Button
            type='submit'
            size='sm'
            disabled={pending || (!!props.grant && !dirty)}
          >
            {t(
              props.grant
                ? 'channels.catalog.saveGrant'
                : 'channels.catalog.addGrant'
            )}
          </Button>
        </div>
        {form.formState.errors.root && (
          <p role='alert' className='text-destructive text-sm'>
            {form.formState.errors.root.message}
          </p>
        )}
      </form>
    </Form>
  )
}
