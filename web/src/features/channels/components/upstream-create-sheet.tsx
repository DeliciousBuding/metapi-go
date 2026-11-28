import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { isAxiosError } from 'axios'
import { KeyRound } from 'lucide-react'
import { useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'
import { api } from '@/lib/api'
import type { UpstreamPreset } from '@/lib/api/upstream-presets'
import { toast } from '@/lib/toast'

import { upstreamKeys } from '../lib/upstream-config'
import {
  upstreamCreateDefaults,
  upstreamCreatePayload,
  upstreamCreateSchema,
  type UpstreamCreateValues,
} from '../lib/upstream-create'
import { UpstreamCreateAdvancedFields } from './upstream-create-advanced-fields'
import { UpstreamCreateCredentialFields } from './upstream-create-credential-fields'
import { UpstreamCreateEndpoints } from './upstream-create-endpoints'
import { UpstreamCreatePresets } from './upstream-create-presets'

export function UpstreamCreateSheet(props: {
  onClose: () => void
  onCreated: (id: number) => void
}) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [selected, setSelected] = useState<UpstreamPreset | null>(null)
  const [advanced, setAdvanced] = useState(false)
  const [saving, setSaving] = useState(false)
  const [resolvingEndpoints, setResolvingEndpoints] = useState(false)
  const submitting = useRef(false)
  const contentRef = useRef<HTMLDivElement>(null)
  const form = useForm<UpstreamCreateValues>({
    resolver: zodResolver(upstreamCreateSchema(selected)),
    defaultValues: upstreamCreateDefaults,
  })
  const pending = saving || form.formState.isSubmitting
  const credentialMode = selected?.credentialMode ?? 'apiKey'
  const requiresBaseUrl = selected?.requiresBaseUrl ?? !selected?.defaultUrl
  const { handleOpenChange, guard } = useDirtyDialogClose({
    enabled: form.formState.isDirty,
    onDiscard: () => form.reset(),
    onOpenChange: (open) => {
      if (!open) props.onClose()
    },
  })
  function selectPreset(preset: UpstreamPreset) {
    if (selected?.id === preset.id) return
    if (contentRef.current) contentRef.current.scrollTop = 0
    setSelected(preset)
    setAdvanced(false)
    form.reset({
      ...upstreamCreateDefaults,
      presetId: preset.id,
      protocols: [...preset.protocols],
      baseUrl: preset.defaultUrl,
    })
  }
  async function submit(values: UpstreamCreateValues) {
    if (
      !selected ||
      credentialMode === 'oauth' ||
      submitting.current ||
      resolvingEndpoints
    )
      return
    submitting.current = true
    setSaving(true)
    form.clearErrors('root')
    try {
      // Credential-bearing input stays out of Query and Mutation caches.
      const result = await api.connectUpstream(
        upstreamCreatePayload(values, selected)
      )
      form.reset({
        ...values,
        apiKey: '',
        gcpServiceAccount: '',
        channelProxy: '',
        customHeaders: '',
        paramOverride: '',
      })
      await client.invalidateQueries({ queryKey: upstreamKeys.all })
      await client.invalidateQueries({ queryKey: ['routes'] })
      const message = t(
        `channels.create.connected.${result.discovery.status}`,
        { count: result.modelCount }
      )
      if (result.discovery.status === 'empty') {
        toast.info(message, { description: result.discovery.message })
      } else toast.success(message, { description: result.discovery.message })
      props.onCreated(result.id)
    } catch (error) {
      const message = isAxiosError(error)
        ? error.response?.data?.error
        : undefined
      form.setError('root', {
        message:
          typeof message === 'string' ? message : t('channels.create.failed'),
      })
    } finally {
      submitting.current = false
      setSaving(false)
    }
  }
  return (
    <>
      <Sheet
        open
        onOpenChange={(open) => {
          if (!pending) handleOpenChange(open)
        }}
      >
        <SheetContent
          className='flex w-full flex-col gap-0 p-0 sm:max-w-2xl'
          showMobileCloseBar={false}
        >
          <SheetHeader className='shrink-0 border-b p-5 pr-12'>
            <SheetTitle>{t('channels.create.title')}</SheetTitle>
            <SheetDescription className='sr-only'>
              {t('channels.create.description')}
            </SheetDescription>
          </SheetHeader>
          <Form {...form}>
            <form
              className='flex min-h-0 flex-1 flex-col'
              onSubmit={form.handleSubmit(submit, (errors) => {
                if (
                  (!requiresBaseUrl && errors.baseUrl) ||
                  errors.name ||
                  errors.channelProxy ||
                  errors.customHeaders ||
                  errors.paramOverride ||
                  errors.providerBinding
                ) {
                  setAdvanced(true)
                }
              })}
            >
              <div
                ref={contentRef}
                className='min-h-0 flex-1 space-y-5 overflow-y-auto p-5'
              >
                <UpstreamCreatePresets
                  selectedId={selected?.id ?? null}
                  pending={pending}
                  onSelect={selectPreset}
                />
                {selected && credentialMode !== 'oauth' && (
                  <div className='space-y-5 border-t pt-5'>
                    <UpstreamCreateCredentialFields
                      selected={selected}
                      pending={pending}
                      requiresBaseUrl={requiresBaseUrl}
                    />
                    <UpstreamCreateEndpoints
                      key={selected.id}
                      preset={selected}
                      disabled={pending}
                      onResolvingChange={setResolvingEndpoints}
                    />
                    <UpstreamCreateAdvancedFields
                      selected={selected}
                      pending={pending}
                      requiresBaseUrl={requiresBaseUrl}
                      advanced={advanced}
                      setAdvanced={setAdvanced}
                      resolvingEndpoints={resolvingEndpoints}
                    />
                  </div>
                )}
                {form.formState.errors.root && (
                  <p role='alert' className='text-destructive text-sm'>
                    {form.formState.errors.root.message}
                  </p>
                )}
              </div>
              <SheetFooter className='shrink-0 flex-row justify-end gap-2 border-t p-5'>
                <Button
                  type='button'
                  variant='outline'
                  disabled={pending}
                  onClick={() => handleOpenChange(false)}
                >
                  {t('common.cancel')}
                </Button>
                {selected && credentialMode === 'oauth' ? (
                  <Button nativeButton={false} render={<Link to='/oauth' />}>
                    <KeyRound aria-hidden />
                    {t('channels.create.authorize')}
                  </Button>
                ) : (
                  <Button
                    type='submit'
                    disabled={!selected || pending || resolvingEndpoints}
                  >
                    {pending && <Spinner aria-hidden />}
                    {t(
                      pending
                        ? 'channels.create.connecting'
                        : 'channels.create.submit'
                    )}
                  </Button>
                )}
              </SheetFooter>
            </form>
          </Form>
        </SheetContent>
      </Sheet>
      {guard}
    </>
  )
}
