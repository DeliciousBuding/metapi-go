// metapi-go/features/settings/sections/downstream — key create/edit sheet
// form: model policy editor (rules + suggestions), site scope picker, and
// the RHF sheet body. Split out of keys-section.tsx (S8 giant-file
// teardown); behavior is unchanged.
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { X } from 'lucide-react'
import { useEffect, useMemo, useState, type ComponentProps } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import type { CredentialExportTarget } from '@/components/common/credential-export-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { useSites } from '@/features/sites'
import { api } from '@/lib/api'
import { toast } from '@/lib/toast'

import { parseIdArray, serializeCredentialRefs } from '../lib/credential-refs'
import { CredentialRefPicker } from './credential-ref-picker'
import {
  CREATE_FORM_ID,
  blankKeyFormValues,
  createKeySchema,
  downstreamKeysQueryKeys,
  editKeySchema,
  generateDownstreamSkSuffix,
  keyFormValuesFromItem,
  normalizeModelRules,
  resolveApiErrorMessage,
  type CreateDownstreamKeyResponse,
  type CreateKeyFormValues,
  type DownstreamApiKeyItem,
} from './key-form-shared'

type ModelPolicyEditorProps = {
  value: string[]
  onChange: (rules: string[]) => void
  candidateModels?: string[]
  routeGrantCount?: number
} & Omit<
  ComponentProps<typeof Input>,
  'value' | 'defaultValue' | 'onChange' | 'onKeyDown'
>

function ModelPolicyEditor({
  value,
  onChange,
  candidateModels = [],
  routeGrantCount = 0,
  ...inputProps
}: ModelPolicyEditorProps) {
  const { t } = useTranslation()
  const [pendingRule, setPendingRule] = useState('')
  const rules = normalizeModelRules(value)
  const normalizedPendingRule = pendingRule.trim()
  const suggestions = useMemo(() => {
    if (!normalizedPendingRule) return []
    const query = normalizedPendingRule.toLowerCase()
    return candidateModels
      .filter(
        (model) => model.toLowerCase().includes(query) && !rules.includes(model)
      )
      .slice(0, 8)
  }, [candidateModels, normalizedPendingRule, rules])

  function addRule(rawRule: string) {
    const rule = rawRule.trim()
    if (!rule) return
    onChange(normalizeModelRules([...rules, rule]))
    setPendingRule('')
  }

  function removeRule(rule: string) {
    onChange(rules.filter((candidate) => candidate !== rule))
  }

  let summary = t('settings.downstream.keys.models.summary.rules', {
    count: rules.length,
    defaultValue: '{{count}} rules',
  })
  let summaryVariant: 'success' | 'warning' | 'outline' = 'outline'
  if (rules.includes('*')) {
    summary = t('settings.downstream.keys.models.summary.all', {
      defaultValue: 'All models',
    })
    summaryVariant = 'success'
  } else if (rules.length === 0 && routeGrantCount > 0) {
    summary = t('settings.downstream.keys.models.summary.routes', {
      count: routeGrantCount,
      defaultValue: '{{count}} route grants',
    })
  } else if (rules.length === 0) {
    summary = t('settings.downstream.keys.models.summary.none', {
      defaultValue: 'No models authorized',
    })
    summaryVariant = 'warning'
  }

  return (
    <div className='space-y-2'>
      <div className='flex flex-wrap items-center gap-2'>
        <Badge variant={summaryVariant} data-testid='model-policy-form-summary'>
          {summary}
        </Badge>
        <Button
          type='button'
          variant='outline'
          size='xs'
          onClick={() => onChange(['*'])}
        >
          {t('settings.downstream.keys.models.allowAll', {
            defaultValue: 'Allow all',
          })}
        </Button>
        <Button
          type='button'
          variant='outline'
          size='xs'
          onClick={() => onChange([])}
        >
          {t('settings.downstream.keys.models.denyAll', {
            defaultValue: 'Deny all',
          })}
        </Button>
      </div>

      {rules.length > 0 ? (
        <div className='flex flex-wrap gap-1' data-testid='model-policy-rules'>
          {rules.map((rule) => (
            <Badge key={rule} variant='secondary' className='font-mono'>
              {rule}
              <button
                type='button'
                className='focus-visible:ring-focus-ring rounded-full outline-none focus-visible:ring-2'
                aria-label={t(
                  'settings.downstream.keys.models.removeRuleAria',
                  { rule, defaultValue: 'Remove model rule {{rule}}' }
                )}
                onClick={() => removeRule(rule)}
              >
                <X aria-hidden='true' />
              </button>
            </Badge>
          ))}
        </div>
      ) : null}

      <div className='flex gap-2'>
        <Input
          {...inputProps}
          value={pendingRule}
          onChange={(event) => setPendingRule(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') {
              event.preventDefault()
              addRule(pendingRule)
            }
          }}
          placeholder={t('settings.downstream.keys.models.inputPlaceholder', {
            defaultValue: 'gpt-5.5, gpt-*, or re:^claude-',
          })}
          className='font-mono'
        />
        <Button
          type='button'
          variant='outline'
          size='sm'
          disabled={!normalizedPendingRule}
          onClick={() => addRule(pendingRule)}
        >
          {t('settings.common.add')}
        </Button>
      </div>

      {suggestions.length > 0 ? (
        <div
          className='flex flex-wrap gap-1'
          role='listbox'
          aria-label={t('settings.downstream.keys.models.suggestions', {
            defaultValue: 'Matching models',
          })}
        >
          {suggestions.map((model) => (
            <Button
              key={model}
              type='button'
              role='option'
              aria-selected='false'
              variant='ghost'
              size='xs'
              className='font-mono'
              onClick={() => addRule(model)}
            >
              + {model}
            </Button>
          ))}
        </div>
      ) : null}
    </div>
  )
}

type SiteScopePickerProps = {
  value: number[]
  onChange: (siteIds: number[]) => void
  sites: Array<{ id: number; name: string }>
}

// Checkbox list over the upstream sites: empty selection means "no site
// restriction" (the routing selector treats an empty allow-list as
// unrestricted), mirroring the model-policy empty-means-deny contrast.
function SiteScopePicker({ value, onChange, sites }: SiteScopePickerProps) {
  const selected = new Set(value)
  return (
    <div
      className='border-border max-h-40 space-y-1 overflow-y-auto rounded-md border p-2'
      data-testid='site-scope-picker'
    >
      {sites.length === 0
        ? null
        : sites.map((site) => (
            <label
              key={site.id}
              className='flex cursor-pointer items-center gap-2 text-sm'
            >
              <input
                type='checkbox'
                checked={selected.has(site.id)}
                onChange={(event) => {
                  const next = new Set(selected)
                  if (event.target.checked) {
                    next.add(site.id)
                  } else {
                    next.delete(site.id)
                  }
                  onChange([...next].sort((left, right) => left - right))
                }}
              />
              {site.name}
            </label>
          ))}
    </div>
  )
}

// Exported so the edit-mode behavior test can render it in isolation,
// mirroring the AccountsRowActions export pattern. The whole KeysSection
// remains the only public entry in the settings surface.
//
// `onCreated` (create mode only) receives the created key's export-dialog
// target straight from the create response so KeysSection can auto-open the
// Connect surface — the operator no longer has to hunt the row down.
export function KeySheetForm({
  editingKey,
  onDone,
  onCreated,
  onDirtyChange,
  candidateModels = [],
}: {
  editingKey: DownstreamApiKeyItem | null
  onDone: () => void
  onCreated?: (target: CredentialExportTarget) => void
  /** Reports RHF dirty state so the hosting Sheet can guard dirty closes. */
  onDirtyChange?: (dirty: boolean) => void
  candidateModels?: string[]
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isEdit = editingKey !== null
  const { data: sites } = useSites()

  const form = useForm<CreateKeyFormValues>({
    resolver: zodResolver(
      editingKey ? editKeySchema : createKeySchema
    ) as never,
    defaultValues: editingKey
      ? keyFormValuesFromItem(editingKey)
      : blankKeyFormValues(),
  })

  // Keep the parent's dirty flag in sync; clearing it on unmount guarantees
  // a reopened sheet never inherits a stale "unsaved" state.
  const isFormDirty = form.formState.isDirty
  useEffect(() => {
    onDirtyChange?.(isFormDirty)
    return () => onDirtyChange?.(false)
  }, [isFormDirty, onDirtyChange])

  function generateKey() {
    form.setValue('key', `sk-${generateDownstreamSkSuffix()}`, {
      shouldDirty: true,
    })
  }

  const submitMutation = useMutation({
    mutationFn: async (values: CreateKeyFormValues) => {
      // Canonical wire format for the credential-ref dimensions: real arrays
      // (create/update bodies never carry the stored JSON-string form).
      const policy = {
        allowedCredentialRefs: serializeCredentialRefs(
          values.allowedCredentialRefs
        ),
        excludedCredentialRefs: serializeCredentialRefs(
          values.excludedCredentialRefs
        ),
      }
      if (!editingKey) {
        return api.createDownstreamApiKey({ ...values, ...policy })
      }
      return api.updateDownstreamApiKey(editingKey.id, {
        name: values.name,
        groupName: values.groupName,
        maxRequests: values.maxRequests,
        maxCost: values.maxCost,
        enabled: values.enabled,
        expiresAt: values.expiresAt,
        supportedModels: values.supportedModels,
        allowedSiteIds: values.allowedSiteIds,
        ...policy,
      })
    },
    onSuccess: (result, values) => {
      void queryClient.invalidateQueries({
        queryKey: downstreamKeysQueryKeys.all,
      })
      toast.success(
        isEdit
          ? t('settings.downstream.keys.toast.updated')
          : t('settings.downstream.keys.toast.created')
      )
      if (!isEdit) {
        // Auto-open the Connect dialog for the freshly created key. The
        // dialog target only needs id/name/keyMasked, which the create
        // response carries under `item`; the plaintext key itself is pulled
        // by the dialog's own export query. When the response lacks the row
        // (older/unexpected payloads) nothing is fabricated — the success
        // toast above stays the only feedback.
        const createdItem = (result as CreateDownstreamKeyResponse | undefined)
          ?.item
        if (createdItem && createdItem.id > 0) {
          onCreated?.({
            id: createdItem.id,
            name: createdItem.name || values.name,
            keyMasked: createdItem.keyMasked,
          })
        }
      }
      onDone()
    },
    onError: (error) => {
      // skipErrorHandler owns this call's feedback: surface the backend's
      // own message when present (400s carry the offending credentialRefs
      // entry index / unknown id), fall back to the generic toast.
      const serverMessage = resolveApiErrorMessage(error)
      toast.error(
        serverMessage ??
          (isEdit
            ? t('settings.downstream.keys.toast.updateFailed')
            : t('settings.downstream.keys.toast.createFailed'))
      )
    },
  })

  function onSubmit(values: CreateKeyFormValues) {
    submitMutation.mutate(values)
  }

  function resolveSubmitLabel(): string {
    if (submitMutation.isPending) return t('settings.common.saving')
    if (isEdit) return t('settings.common.save')
    return t('settings.common.create')
  }

  return (
    <>
      <SheetHeader>
        <SheetTitle>
          {isEdit
            ? t('settings.downstream.keys.editTitle')
            : t('settings.downstream.keys.createTitle')}
        </SheetTitle>
        <SheetDescription>
          {isEdit
            ? t('settings.downstream.keys.editDescription')
            : t('settings.downstream.keys.createDescription')}
        </SheetDescription>
      </SheetHeader>
      <Form {...form}>
        <form
          id={CREATE_FORM_ID}
          onSubmit={form.handleSubmit(onSubmit)}
          className='min-h-0 flex-1 space-y-4 overflow-y-auto px-4'
        >
          <FormField
            control={form.control}
            name='name'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('settings.downstream.keys.fields.name')}
                </FormLabel>
                <FormControl>
                  <Input {...field} value={field.value ?? ''} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          {isEdit ? null : (
            <FormField
              control={form.control}
              name='key'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('settings.downstream.keys.fields.key')}
                  </FormLabel>
                  <div className='flex gap-2'>
                    <FormControl>
                      <Input
                        {...field}
                        value={field.value ?? ''}
                        className='font-mono'
                        placeholder='sk-…'
                      />
                    </FormControl>
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      onClick={generateKey}
                    >
                      {t('settings.downstream.keys.generate')}
                    </Button>
                  </div>
                  <FormDescription>
                    {t('settings.downstream.keys.fields.keyHint')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}
          <FormField
            control={form.control}
            name='supportedModels'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('settings.downstream.keys.models.fieldLabel', {
                    defaultValue: 'Model access',
                  })}
                </FormLabel>
                <FormDescription>
                  {t('settings.downstream.keys.models.hint', {
                    defaultValue:
                      'Empty denies all. Add exact names, glob patterns (*), or re: regex rules. All models requires an explicit *.',
                  })}
                </FormDescription>
                <FormControl>
                  <ModelPolicyEditor
                    value={field.value ?? []}
                    onChange={field.onChange}
                    candidateModels={candidateModels}
                    routeGrantCount={
                      parseIdArray(editingKey?.allowedRouteIds).length
                    }
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='allowedSiteIds'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('settings.downstream.keys.sites.fieldLabel', {
                    defaultValue: 'Upstream site restriction',
                  })}
                </FormLabel>
                <FormDescription>
                  {t('settings.downstream.keys.sites.hint', {
                    defaultValue:
                      'Leave empty for no site restriction. Selecting sites limits this key to channels of the chosen upstream sites.',
                  })}
                </FormDescription>
                <FormControl>
                  <SiteScopePicker
                    value={field.value ?? []}
                    onChange={field.onChange}
                    sites={sites ?? []}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='allowedCredentialRefs'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('settings.downstream.keys.credentials.fieldLabelAllow')}
                </FormLabel>
                <FormDescription>
                  {t('settings.downstream.keys.credentials.hintAllow')}
                </FormDescription>
                <FormControl>
                  <CredentialRefPicker
                    value={field.value ?? []}
                    onChange={field.onChange}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='excludedCredentialRefs'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('settings.downstream.keys.credentials.fieldLabelExclude')}
                </FormLabel>
                <FormDescription>
                  {t('settings.downstream.keys.credentials.hintExclude')}
                </FormDescription>
                <FormControl>
                  <CredentialRefPicker
                    value={field.value ?? []}
                    onChange={field.onChange}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='groupName'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('settings.downstream.keys.fields.groupName')}
                </FormLabel>
                <FormControl>
                  <Input {...field} value={field.value ?? ''} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <div className='grid grid-cols-2 gap-4'>
            <FormField
              control={form.control}
              name='maxRequests'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('settings.downstream.keys.fields.maxRequests')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      value={field.value ?? ''}
                      type='number'
                      min={0}
                      placeholder={t('settings.common.unlimited')}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='maxCost'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('settings.downstream.keys.fields.maxCost')}
                  </FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      value={field.value ?? ''}
                      type='number'
                      min={0}
                      step={0.01}
                      placeholder={t('settings.common.unlimited')}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>
          <FormField
            control={form.control}
            name='expiresAt'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('settings.downstream.keys.fields.expiresAt')}
                </FormLabel>
                <FormControl>
                  <Input
                    {...field}
                    value={field.value ?? ''}
                    type='datetime-local'
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <FormItem className='flex flex-row items-center gap-3'>
                <FormControl>
                  <Switch
                    checked={Boolean(field.value)}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
                <FormLabel className='cursor-pointer'>
                  {t('settings.downstream.keys.fields.enabled')}
                </FormLabel>
              </FormItem>
            )}
          />
          {isEdit ? null : (
            <FormField
              control={form.control}
              name='description'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('settings.downstream.keys.fields.description')}
                  </FormLabel>
                  <FormControl>
                    <Input {...field} value={field.value ?? ''} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}
        </form>
      </Form>
      <SheetFooter>
        <Button
          type='submit'
          form={CREATE_FORM_ID}
          disabled={submitMutation.isPending}
        >
          {resolveSubmitLabel()}
        </Button>
      </SheetFooter>
    </>
  )
}
