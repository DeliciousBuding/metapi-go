// metapi-go/features/settings/sections/downstream/components — downstream
// keys section. A lean list + create sheet + enable/disable/delete actions.
// The legacy DownstreamKeys page (1500+ lines, rich editor, batch ops, trend
// charts) is intentionally reduced to its core here; richer surfaces can be
// layered back on as separate sub-features once the rewrite matures.

import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { ColumnDef } from '@tanstack/react-table'
import { Pencil, X } from 'lucide-react'
import { useEffect, useMemo, useState, type ComponentProps } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import {
  CredentialExportDialog,
  type CredentialExportTarget,
} from '@/components/common/credential-export-dialog'
import { DataTablePage, useDataTable } from '@/components/data-table'
import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { useAccounts, useAllAccountTokens } from '@/features/accounts'
import { useSites } from '@/features/sites'
import { api } from '@/lib/api'
import { toast } from '@/lib/toast'

import { SettingsSectionCard } from '../../../components/settings-section-card'
import { SettingsSectionError } from '../../../components/settings-section-error'
import { accountDisplayName, tokenDisplayName } from '../lib/credential-display'
import {
  credentialRefSchema,
  parseCredentialRefs,
  parseIdArray,
  serializeCredentialRefs,
} from '../lib/credential-refs'
import { CredentialRefPicker } from './credential-ref-picker'
import { KeyScopeCell, type ScopeNameMaps } from './key-scope-cell'

type DownstreamKeyUsage24h = {
  requests?: number
  tokens?: number
  cost?: number
}

type DownstreamApiKeyItem = {
  id: number
  name: string
  keyMasked?: string
  groupName?: string
  enabled: boolean
  expiresAt?: string | null
  maxCost?: number | null
  usedCost?: number | null
  maxRequests?: number | null
  usedRequests?: number | null
  supportedModels?: string[] | string | null
  allowedRouteIds?: number[] | string | null
  allowedSiteIds?: number[] | string | null
  excludedSiteIds?: number[] | string | null
  // Credential-ref columns: GET returns the stored columns verbatim — a raw
  // JSON string (or null); parsed with parseCredentialRefs before use.
  allowedCredentialRefs?: string | unknown[] | null
  excludedCredentialRefs?: string | unknown[] | null
  usage24h?: DownstreamKeyUsage24h
}

type DownstreamKeysResponse = { items: DownstreamApiKeyItem[] }

// POST /api/downstream-keys responds with the created row under `item` (the
// handler re-reads the inserted row and adds a camelCase `keyMasked`). Only
// the fields the connect dialog target needs are typed here; the dialog
// fetches the full export payload (endpoint + plaintext key) on its own.
type CreateDownstreamKeyResponse = {
  success?: boolean
  item?: Pick<DownstreamApiKeyItem, 'id' | 'name' | 'keyMasked'>
}

const downstreamKeysQueryKeys = {
  all: ['downstream-keys'] as const,
  list: () => [...downstreamKeysQueryKeys.all, 'list'] as const,
}

const CREATE_FORM_ID = 'settings-downstream-keys-create-form'

// Extract the backend error message from an axios rejection. Admin API
// errors serialize as { error: "..." } (handler/shared/errors.go APIError);
// .message is checked too, mirroring the http-client resolveResponseMessage
// order. Returns undefined for shapes without a usable string.
function resolveApiErrorMessage(error: unknown): string | undefined {
  const data = (error as { response?: { data?: unknown } } | null)?.response
    ?.data
  if (data && typeof data === 'object') {
    const message = (data as { message?: unknown }).message
    if (typeof message === 'string' && message.trim()) return message
    const errorField = (data as { error?: unknown }).error
    if (typeof errorField === 'string' && errorField.trim()) return errorField
  }
  return undefined
}

function generateDownstreamSkSuffix(): string {
  const bytes = new Uint8Array(48)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join(
    ''
  )
}

const createKeySchema = z.object({
  name: z.string().min(1, 'settings.downstream.keys.schema.nameRequired'),
  key: z.string().min(8, 'settings.downstream.keys.schema.keyMinLength'),
  groupName: z.string().optional(),
  maxRequests: z.coerce.number().int().min(0).optional(),
  maxCost: z.coerce.number().min(0).optional(),
  enabled: z.boolean().optional(),
  expiresAt: z.string().optional(),
  description: z.string().optional(),
  supportedModels: z.array(z.string().trim().min(1)).default([]),
  allowedSiteIds: z.array(z.number().int().positive()).default([]),
  allowedCredentialRefs: z.array(credentialRefSchema).default([]),
  excludedCredentialRefs: z.array(credentialRefSchema).default([]),
})

type CreateKeyFormValues = z.infer<typeof createKeySchema>

// Edit mode reuses the create schema but drops the secret `key` field — the
// key value is never editable here (rotation is a separate action). The
// backend applies a PATCH-style partial update (only fields present in the
// request body are changed; the toggle path already relies on this), so the
// edit payload omits `key` and `description` to preserve both as-is.
const editKeySchema = createKeySchema.omit({ key: true })

// datetime-local input format: "YYYY-MM-DDTHH:MM" in the browser's local
// timezone. The backend stores the value as-is when it cannot parse it as
// RFC3339, so create/edit round-trip the same local string.
function isoToLocalDatetimeInput(iso?: string | null): string {
  if (!iso) return ''
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function normalizeModelRules(value: unknown): string[] {
  let rawRules: unknown[] = []
  if (Array.isArray(value)) {
    rawRules = value
  } else if (typeof value === 'string' && value.trim()) {
    try {
      const parsed = JSON.parse(value) as unknown
      rawRules = Array.isArray(parsed) ? parsed : [value]
    } catch {
      rawRules = [value]
    }
  }

  const rules: string[] = []
  const seen = new Set<string>()
  for (const rawRule of rawRules) {
    if (typeof rawRule !== 'string') continue
    const rule = rawRule.trim()
    if (!rule || seen.has(rule)) continue
    if (rule === '*') return ['*']
    seen.add(rule)
    rules.push(rule)
  }
  return rules
}

function extractMarketplaceModelNames(result: unknown): string[] {
  let rows: unknown = []
  if (Array.isArray(result)) {
    rows = result
  } else if (
    typeof result === 'object' &&
    result !== null &&
    'models' in result
  ) {
    rows = (result as { models?: unknown }).models
  }
  if (!Array.isArray(rows)) return []

  return normalizeModelRules(
    rows.map((row) =>
      typeof row === 'object' && row !== null && 'name' in row
        ? (row as { name?: unknown }).name
        : undefined
    )
  )
}

function blankKeyFormValues(): CreateKeyFormValues {
  return {
    name: '',
    key: '',
    groupName: '',
    maxRequests: undefined,
    maxCost: undefined,
    enabled: true,
    expiresAt: '',
    description: '',
    supportedModels: [],
    allowedSiteIds: [],
    allowedCredentialRefs: [],
    excludedCredentialRefs: [],
  }
}

function keyFormValuesFromItem(
  item: DownstreamApiKeyItem
): CreateKeyFormValues {
  return {
    ...blankKeyFormValues(),
    name: item.name,
    groupName: item.groupName ?? '',
    maxRequests: item.maxRequests ?? undefined,
    maxCost: item.maxCost ?? undefined,
    enabled: item.enabled,
    expiresAt: isoToLocalDatetimeInput(item.expiresAt),
    supportedModels: normalizeModelRules(item.supportedModels),
    allowedSiteIds: parseIdArray(item.allowedSiteIds),
    // GET returns the stored ref columns as raw JSON strings — parse them
    // back into typed refs for the tree pickers (round-trip contract).
    allowedCredentialRefs: parseCredentialRefs(item.allowedCredentialRefs),
    excludedCredentialRefs: parseCredentialRefs(item.excludedCredentialRefs),
  }
}

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

export function KeyModelPolicyCell({
  supportedModels,
  allowedRouteIds,
}: {
  supportedModels?: DownstreamApiKeyItem['supportedModels']
  allowedRouteIds?: DownstreamApiKeyItem['allowedRouteIds']
}) {
  const { t } = useTranslation()
  const rules = normalizeModelRules(supportedModels)
  const routeGrantCount = parseIdArray(allowedRouteIds).length
  if (rules.includes('*')) {
    return (
      <Badge variant='success'>
        {t('settings.downstream.keys.models.summary.all', {
          defaultValue: 'All models',
        })}
      </Badge>
    )
  }
  if (rules.length === 0 && routeGrantCount > 0) {
    return (
      <Badge variant='outline'>
        {t('settings.downstream.keys.models.summary.routes', {
          count: routeGrantCount,
          defaultValue: '{{count}} route grants',
        })}
      </Badge>
    )
  }
  if (rules.length === 0) {
    return (
      <Badge variant='warning'>
        {t('settings.downstream.keys.models.summary.none', {
          defaultValue: 'No models authorized',
        })}
      </Badge>
    )
  }
  return (
    <Badge variant='outline'>
      {t('settings.downstream.keys.models.summary.rules', {
        count: rules.length,
        defaultValue: '{{count}} rules',
      })}
    </Badge>
  )
}

// Exported so the usage-cell test can render it in isolation (same pattern as
// KeySheetForm). Renders quota usage plus the per-key 24h proxy_logs summary.
export function KeyUsageCell({ item }: { item: DownstreamApiKeyItem }) {
  const { t } = useTranslation()
  return (
    <div>
      <div>
        {t('settings.downstream.keys.requests', {
          used: item.usedRequests ?? 0,
          max: item.maxRequests ?? t('settings.common.unlimited'),
        })}
      </div>
      <div>
        {t('settings.downstream.keys.cost', {
          used: item.usedCost ?? 0,
          max: item.maxCost ?? t('settings.common.unlimited'),
        })}
      </div>
      <div className='mt-1 border-t pt-1'>
        {t('settings.downstream.keys.usage24h', {
          requests: item.usage24h?.requests ?? 0,
          tokens: item.usage24h?.tokens ?? 0,
          cost: item.usage24h?.cost ?? 0,
        })}
      </div>
    </div>
  )
}

export function KeysSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [formDirty, setFormDirty] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<DownstreamApiKeyItem | null>(
    null
  )
  const [exportTarget, setExportTarget] =
    useState<CredentialExportTarget | null>(null)

  const keysQuery = useQuery<DownstreamKeysResponse>({
    queryKey: downstreamKeysQueryKeys.list(),
    queryFn: async () =>
      (await api.getDownstreamApiKeys()) as DownstreamKeysResponse,
    staleTime: 15 * 1000,
  })

  const modelInventoryQuery = useQuery<unknown>({
    queryKey: [
      'models',
      'marketplace',
      { refresh: false, includePricing: false },
    ],
    queryFn: () => api.getModelsMarketplace(),
    enabled: createOpen,
    staleTime: 60 * 1000,
  })
  const candidateModels = useMemo(
    () => extractMarketplaceModelNames(modelInventoryQuery.data),
    [modelInventoryQuery.data]
  )

  // Routing-scope display resolves raw site/account/token IDs to names.
  // These share their query keys with the sites/accounts/tokens surfaces
  // (and with the credential pickers inside the edit sheet), so the list
  // pays at most one extra fetch per inventory dimension.
  const scopeSitesQuery = useSites()
  const scopeAccountsQuery = useAccounts()
  const scopeTokensQuery = useAllAccountTokens()
  const scopeNames = useMemo<ScopeNameMaps>(
    () => ({
      sites: new Map(
        (scopeSitesQuery.data ?? []).map((site) => [site.id, site.name])
      ),
      accounts: new Map(
        (scopeAccountsQuery.data?.accounts ?? []).map((account) => [
          account.id,
          accountDisplayName(account),
        ])
      ),
      tokens: new Map(
        (scopeTokensQuery.data ?? []).map((token) => [
          token.id,
          tokenDisplayName(token),
        ])
      ),
    }),
    [scopeSitesQuery.data, scopeAccountsQuery.data, scopeTokensQuery.data]
  )

  const [editingKey, setEditingKey] = useState<DownstreamApiKeyItem | null>(
    null
  )

  function openCreate() {
    setEditingKey(null)
    setCreateOpen(true)
  }

  function openEdit(item: DownstreamApiKeyItem) {
    setEditingKey(item)
    setCreateOpen(true)
  }

  function onSheetOpenChange(open: boolean) {
    setCreateOpen(open)
    if (!open) {
      setEditingKey(null)
    }
  }

  // User-initiated closes (X / Escape / overlay) are intercepted while the
  // form is dirty; the post-save close path calls onSheetOpenChange directly
  // on purpose so a successful submit never trips the discard prompt.
  const { handleOpenChange: guardedSheetOpenChange, guard: sheetDirtyGuard } =
    useDirtyDialogClose({
      enabled: formDirty,
      onDiscard: () => setFormDirty(false),
      onOpenChange: onSheetOpenChange,
    })

  const toggleMutation = useMutation({
    mutationFn: async ({ id, enabled }: { id: number; enabled: boolean }) =>
      api.updateDownstreamApiKey(id, { enabled }),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: downstreamKeysQueryKeys.all,
      })
      toast.success(t('settings.downstream.keys.toast.updated'))
    },
    onError: () =>
      toast.error(t('settings.downstream.keys.toast.updateFailed')),
  })

  const deleteMutation = useMutation({
    mutationFn: async (id: number) => api.deleteDownstreamApiKey(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: downstreamKeysQueryKeys.all,
      })
      toast.success(t('settings.downstream.keys.toast.deleted'))
      setDeleteTarget(null)
    },
    onError: () =>
      toast.error(t('settings.downstream.keys.toast.deleteFailed')),
  })

  const items = keysQuery.data?.items ?? []
  const isLoading = keysQuery.isLoading

  // Stable top-level refs for the columns memo (same pattern as the
  // accounts page's toggleStatusMutate): the mutation object's identity
  // changes every render, the memo only needs the stable mutate fn plus the
  // derived pending flag.
  const toggleKeyMutate = toggleMutation.mutate
  const toggleKeyPending = toggleMutation.isPending

  // One column set serves the desktop table and the ≤640px card list — the
  // same DataTablePage contract every list page uses: `mobileTitle` lifts
  // the key name into the card header, `mobileBadge` docks the enable
  // switch beside it, and the `actions` column id renders the
  // Connect/Edit/Delete buttons at the card bottom. Before this migration
  // the section rendered a bare <Table> that horizontally scrolled the
  // whole row set on phones, pushing Connect/Delete off-screen.
  const columns = useMemo<ColumnDef<DownstreamApiKeyItem, unknown>[]>(
    () => [
      {
        id: 'name',
        header: t('settings.downstream.keys.columns.name'),
        meta: { mobileTitle: true },
        cell: ({ row }) => (
          <div className='flex flex-col'>
            <span className='font-medium'>{row.original.name}</span>
            {row.original.keyMasked ? (
              <code className='text-muted-foreground text-xs'>
                {row.original.keyMasked}
              </code>
            ) : null}
          </div>
        ),
      },
      {
        id: 'group',
        header: t('settings.downstream.keys.columns.group'),
        cell: ({ row }) =>
          row.original.groupName ? (
            <Badge variant='secondary'>{row.original.groupName}</Badge>
          ) : (
            <span className='text-muted-foreground'>—</span>
          ),
      },
      {
        id: 'models',
        header: t('settings.downstream.keys.columns.models', {
          defaultValue: 'Models',
        }),
        cell: ({ row }) => (
          <KeyModelPolicyCell
            supportedModels={row.original.supportedModels}
            allowedRouteIds={row.original.allowedRouteIds}
          />
        ),
      },
      {
        id: 'scope',
        header: t('settings.downstream.keys.columns.scope'),
        cell: ({ row }) => (
          <KeyScopeCell item={row.original} names={scopeNames} />
        ),
      },
      {
        id: 'enabled',
        header: t('settings.downstream.keys.columns.enabled'),
        meta: { mobileBadge: true },
        cell: ({ row }) => (
          <Switch
            checked={row.original.enabled}
            disabled={toggleKeyPending}
            onCheckedChange={(checked) =>
              toggleKeyMutate({ id: row.original.id, enabled: checked })
            }
            aria-label={t('settings.downstream.keys.columns.enabledAria', {
              name: row.original.name,
            })}
          />
        ),
      },
      {
        id: 'usage',
        header: t('settings.downstream.keys.columns.usage'),
        cell: ({ row }) => (
          <div className='text-muted-foreground text-xs'>
            <KeyUsageCell item={row.original} />
          </div>
        ),
      },
      {
        id: 'actions',
        header: t('settings.downstream.keys.columns.actions'),
        cell: ({ row }) => (
          <div className='flex justify-end gap-1'>
            <Button
              type='button'
              variant='ghost'
              size='sm'
              onClick={() => setExportTarget(row.original)}
            >
              {t('settings.downstream.keys.connect')}
            </Button>
            <Button
              type='button'
              variant='ghost'
              size='icon-sm'
              aria-label={t('settings.downstream.keys.columns.editAria', {
                name: row.original.name,
              })}
              onClick={() => openEdit(row.original)}
            >
              <Pencil />
            </Button>
            <Button
              type='button'
              variant='ghost'
              size='sm'
              onClick={() => setDeleteTarget(row.original)}
            >
              {t('settings.common.delete')}
            </Button>
          </div>
        ),
      },
    ],
    [t, toggleKeyPending, toggleKeyMutate, scopeNames]
  )

  const { table } = useDataTable<DownstreamApiKeyItem>({
    data: items,
    columns,
    getRowId: (row) => String(row.id),
  })

  return (
    <SettingsSectionCard
      title={t('settings.downstream.keys.title')}
      description={t('settings.downstream.keys.description')}
      actions={
        <Button size='sm' onClick={() => openCreate()}>
          {t('settings.downstream.keys.create')}
        </Button>
      }
    >
      {keysQuery.isError ? (
        <SettingsSectionError
          title={t('settings.downstream.keys.title')}
          onRetry={() => void keysQuery.refetch()}
        />
      ) : (
        <DataTablePage
          table={table}
          columns={columns}
          isLoading={isLoading}
          emptyTitle={t('settings.downstream.keys.empty')}
          emptyAction={
            <Button size='sm' onClick={() => openCreate()}>
              {t('settings.downstream.keys.create')}
            </Button>
          }
          toolbarProps={null}
          fixedHeight={false}
          skeletonKeyPrefix='downstream-keys-skeleton'
        />
      )}
      {/* The mobile empty state carries no action button (MobileCardList
          contract) — the section header's Create action stays visible on
          every viewport, so the empty flow loses nothing. */}

      <Sheet open={createOpen} onOpenChange={guardedSheetOpenChange}>
        {/* Mobile contract comes from the SheetContent base: full-width panel
            below `sm` + flex-column layout. The form body is the scroll
            region (flex-1), so the submit footer (SheetFooter `mt-auto`)
            stays pinned at the bottom instead of scrolling out of reach. */}
        <SheetContent>
          <KeySheetForm
            key={editingKey?.id ?? 'create'}
            editingKey={editingKey}
            onDone={() => onSheetOpenChange(false)}
            onCreated={(target) => setExportTarget(target)}
            onDirtyChange={setFormDirty}
            candidateModels={candidateModels}
          />
          {sheetDirtyGuard}
        </SheetContent>
      </Sheet>

      <Dialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteTarget(null)
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {t('settings.downstream.keys.deleteTitle')}
            </DialogTitle>
            <DialogDescription>
              {t('settings.downstream.keys.deleteDescription', {
                name: deleteTarget?.name ?? '',
              })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setDeleteTarget(null)}
              disabled={deleteMutation.isPending}
            >
              {t('settings.common.cancel')}
            </Button>
            <Button
              variant='destructive'
              disabled={deleteMutation.isPending}
              onClick={() => {
                if (deleteTarget) {
                  deleteMutation.mutate(deleteTarget.id)
                }
              }}
            >
              {deleteMutation.isPending && <Spinner />}
              {t('settings.common.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <CredentialExportDialog
        target={exportTarget}
        onOpenChange={(open) => {
          if (!open) setExportTarget(null)
        }}
      />
    </SettingsSectionCard>
  )
}
