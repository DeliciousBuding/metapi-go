import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import { api } from '@/lib/api'
import type { ImportedUpstreamDetail } from '@/lib/api/imported-upstreams'
import { toast } from '@/lib/toast'

import {
  connectionPatch,
  connectionSchema,
  connectionValues,
  upstreamKeys,
  type ConnectionValues,
} from '../lib/upstream-config'
import { UpstreamConnectionAdvanced } from './upstream-connection-advanced'
import { UpstreamConnectionEndpoints } from './upstream-connection-endpoints'

export function UpstreamConnectionForm(props: {
  detail: ImportedUpstreamDetail
  onDirtyChange: (key: string, dirty: boolean) => void
  onManageModels?: () => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const form = useForm<ConnectionValues>({
    resolver: zodResolver(connectionSchema),
    defaultValues: connectionValues(props.detail),
  })
  const dirty = form.formState.isDirty
  const pending = form.formState.isSubmitting
  const [resolvingPreset, setResolvingPreset] = useState(false)
  const onDirtyChange = props.onDirtyChange
  useEffect(() => {
    onDirtyChange('connection', dirty)
    return () => onDirtyChange('connection', false)
  }, [dirty, onDirtyChange])

  async function save(values: ConnectionValues) {
    if (resolvingPreset) return false
    const result = await api.updateImportedUpstream(
      props.detail.id,
      connectionPatch(values, form.formState.dirtyFields)
    )
    if (!result.success) return false
    form.reset(values)
    await client.invalidateQueries({ queryKey: upstreamKeys.all })
    toast.success(t('channels.upstream.saved'))
    return true
  }

  function manageModels() {
    if (!dirty) {
      props.onManageModels?.()
      return
    }
    void form
      .handleSubmit(async (values) => {
        if (await save(values)) props.onManageModels?.()
      })()
      .catch(() => {})
  }

  return (
    <Form {...form}>
      <form
        className='space-y-6'
        noValidate
        onSubmit={form.handleSubmit((values) => save(values).catch(() => {}))}
      >
        <UpstreamConnectionEndpoints
          detail={props.detail}
          resolvingPreset={resolvingPreset}
          onResolvingChange={setResolvingPreset}
          onManageModels={props.onManageModels && manageModels}
        />
        <UpstreamConnectionAdvanced detail={props.detail} />
        <div className='bg-background/95 sticky bottom-0 -mx-1 flex justify-end border-t px-1 py-3'>
          <Button type='submit' disabled={!dirty || pending || resolvingPreset}>
            {t('channels.upstream.saveConnection')}
          </Button>
        </div>
      </form>
    </Form>
  )
}
