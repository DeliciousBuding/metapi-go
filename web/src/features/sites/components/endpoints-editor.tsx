// metapi-go/features/sites/components — structured apiEndpoints editor.
//
// Replaces the free-form JSON textarea with per-endpoint rows (URL input +
// enabled switch + up/down order + delete), mirroring the TS original's
// site form endpoint pool (Sites.tsx:1438-1528): list order = polling order,
// and each row surfaces the endpoint's live cooldown / last failure state
// (data already present on `SiteApiEndpoint`, read from the edited site).
//
// The underlying form field stays `apiEndpointsText` (validated by the same
// zod superRefine), so this component is a thin wrapper: it owns the row
// state, serializes it back into the field value on every mutation, and
// reconciles external value changes (dialog resets) exactly once. The
// textarea remains reachable as a collapsed "advanced" mode.

import {
  ArrowDown as ArrowDownIcon,
  ArrowUp as ArrowUpIcon,
} from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { toBcp47 } from '@/i18n/languages'
import { formatRelativeTime } from '@/lib/format'

import {
  normalizeEndpointBaseUrl,
  parseEndpointsEditorText,
} from '../lib/endpoints'
import type { SiteApiEndpoint } from '../types'

type EndpointRow = {
  // Stable identity across reorders (row index is not stable).
  key: string
  url: string
  enabled: boolean
}

type EndpointsEditorProps = {
  value: string
  onChange: (value: string) => void
  /** Live status data for the site being edited (cooldown / failures). */
  liveEndpoints?: SiteApiEndpoint[]
}

function nextRowKey(): string {
  return `row-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

function rowsFromText(text: string): EndpointRow[] {
  const parsed = parseEndpointsEditorText(text)
  if (!('endpoints' in parsed)) return []
  return parsed.endpoints.map((endpoint) => ({
    key: nextRowKey(),
    url: endpoint.url,
    enabled: endpoint.enabled,
  }))
}

/** Serialize rows back to the textarea wire format with positional sortOrder
 *  (list order = polling order, matching the TS original's row editor). */
function rowsToText(rows: EndpointRow[]): string {
  return rows
    .map((row, index) =>
      JSON.stringify({
        url: row.url,
        enabled: row.enabled,
        sortOrder: index,
      })
    )
    .join('\n')
}

function findLiveEndpoint(
  liveEndpoints: SiteApiEndpoint[],
  rowUrl: string
): SiteApiEndpoint | undefined {
  const normalized = normalizeEndpointBaseUrl(rowUrl)
  if (!normalized) return undefined
  return liveEndpoints.find(
    (endpoint) => normalizeEndpointBaseUrl(endpoint.url) === normalized
  )
}

export function EndpointsEditor({
  value,
  onChange,
  liveEndpoints = [],
}: EndpointsEditorProps) {
  const { t, i18n } = useTranslation()
  const [rows, setRows] = useState<EndpointRow[]>(() =>
    rowsFromText(value)
  )
  const [advanced, setAdvanced] = useState(false)
  const [parseBlocked, setParseBlocked] = useState<boolean>(false)
  // Tracks the value this component emitted itself so an echoing value update
  // from RHF never re-derives rows while the operator is typing.
  const lastEmitted = useRef<string | null>(null)

  useEffect(() => {
    if (value === lastEmitted.current) return
    // External change (dialog reset / advanced-mode edit): rebuild rows if
    // the value parses; otherwise block the structured view until fixed.
    const parsed = parseEndpointsEditorText(value)
    if ('endpoints' in parsed) {
      setRows(
        parsed.endpoints.map((endpoint) => ({
          key: nextRowKey(),
          url: endpoint.url,
          enabled: endpoint.enabled,
        }))
      )
      setParseBlocked(false)
    } else {
      setParseBlocked(true)
    }
  }, [value])

  function commit(nextRows: EndpointRow[]) {
    setRows(nextRows)
    const serialized = rowsToText(nextRows)
    lastEmitted.current = serialized
    onChange(serialized)
  }

  function updateRow(index: number, patch: Partial<EndpointRow>) {
    commit(rows.map((row, i) => (i === index ? { ...row, ...patch } : row)))
  }

  function moveRow(index: number, direction: -1 | 1) {
    const target = index + direction
    if (target < 0 || target >= rows.length) return
    const next = [...rows]
    const [row] = next.splice(index, 1) as [EndpointRow]
    next.splice(target, 0, row)
    commit(next)
  }

  function removeRow(index: number) {
    commit(rows.filter((_, i) => i !== index))
  }

  function addRow() {
    commit([...rows, { key: nextRowKey(), url: '', enabled: true }])
  }

  if (advanced) {
    return (
      <div className='space-y-2'>
        <Textarea
          aria-label={t('sites.form.apiEndpointsTextareaLabel')}
          rows={6}
          placeholder='{"url":"https://api.example.com","enabled":true}'
          className='font-mono text-xs'
          value={value}
          onChange={(event) => onChange(event.target.value)}
        />
        <div className='flex items-center justify-between gap-2'>
          <p className='text-muted-foreground text-xs'>
            {t('sites.form.apiEndpointsHint')}
          </p>
          <Button
            type='button'
            variant='link'
            size='xs'
            onClick={() => {
              const parsed = parseEndpointsEditorText(value)
              if (!('endpoints' in parsed)) {
                setParseBlocked(true)
                return
              }
              setParseBlocked(false)
              setAdvanced(false)
            }}
          >
            {t('sites.form.apiEndpointsStructuredBack')}
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div className='space-y-2'>
      {parseBlocked ? (
        <div className='bg-destructive/10 text-destructive rounded-md border p-3 text-xs'>
          {t('sites.form.apiEndpointsParseBlocked')}
        </div>
      ) : (
        <div className='space-y-2'>
          {rows.map((row, index) => {
            const live = findLiveEndpoint(liveEndpoints, row.url)
            const cooldownActive =
              Boolean(live?.cooldownUntil) &&
              new Date(live?.cooldownUntil as string) > new Date()
            const failureReason = live?.lastFailureReason?.trim() || null
            return (
              <div
                key={row.key}
                className='bg-muted/30 rounded-md border p-2 space-y-1.5'
              >
                <div className='flex items-center gap-2'>
                  <Input
                    aria-label={`${t('sites.form.endpointUrlLabel')} ${index + 1}`}
                    placeholder='https://api.example.com'
                    className='flex-1 font-mono text-xs'
                    value={row.url}
                    onChange={(event) =>
                      updateRow(index, { url: event.target.value })
                    }
                  />
                  <div className='text-muted-foreground flex shrink-0 items-center gap-1.5 text-xs'>
                    <Switch
                      size='sm'
                      aria-label={t('sites.form.endpointEnabledLabel', {
                        index: index + 1,
                      })}
                      checked={row.enabled}
                      onCheckedChange={(checked) =>
                        updateRow(index, { enabled: checked })
                      }
                    />
                    {t('sites.form.endpointEnabled')}
                  </div>
                </div>
                <div className='flex items-center justify-between gap-2'>
                  <div className='text-muted-foreground flex flex-wrap items-center gap-x-3 gap-y-1 text-xs'>
                    <span>
                      {t('sites.form.endpointOrder', { index: index + 1 })}
                    </span>
                    {cooldownActive && live?.cooldownUntil && (
                      <span className='flex items-center gap-1'>
                        <Badge variant='warning'>
                          {t('sites.detail.endpointCooldown')}
                        </Badge>
                        {formatRelativeTime(
                          live.cooldownUntil,
                          toBcp47(i18n.language)
                        )}
                      </span>
                    )}
                    {failureReason && (
                      <span
                        className='text-destructive truncate'
                        title={failureReason}
                      >
                        {t('sites.detail.endpointFailureReason')}:{' '}
                        {failureReason}
                      </span>
                    )}
                  </div>
                  <div className='flex shrink-0 items-center gap-1'>
                    <Button
                      type='button'
                      variant='ghost'
                      size='xs'
                      aria-label={t('sites.form.endpointMoveUp', {
                        index: index + 1,
                      })}
                      disabled={index === 0}
                      onClick={() => moveRow(index, -1)}
                    >
                      <ArrowUpIcon className='size-3.5' />
                    </Button>
                    <Button
                      type='button'
                      variant='ghost'
                      size='xs'
                      aria-label={t('sites.form.endpointMoveDown', {
                        index: index + 1,
                      })}
                      disabled={index >= rows.length - 1}
                      onClick={() => moveRow(index, 1)}
                    >
                      <ArrowDownIcon className='size-3.5' />
                    </Button>
                    <Button
                      type='button'
                      variant='ghost'
                      size='xs'
                      aria-label={t('sites.form.endpointRemove', {
                        index: index + 1,
                      })}
                      onClick={() => removeRow(index)}
                    >
                      {t('sites.form.endpointRemoveLabel')}
                    </Button>
                  </div>
                </div>
              </div>
            )
          })}
          {rows.length === 0 && (
            <p className='text-muted-foreground text-xs'>
              {t('sites.form.apiEndpointsEmpty')}
            </p>
          )}
        </div>
      )}
      <p className='text-muted-foreground text-xs'>
        {t('sites.form.apiEndpointsStructuredHint')}
      </p>
      <div className='flex items-center justify-between gap-2'>
        <Button
          type='button'
          variant='outline'
          size='xs'
          onClick={addRow}
          disabled={parseBlocked}
        >
          + {t('sites.form.apiEndpointsAdd')}
        </Button>
        <Button
          type='button'
          variant='link'
          size='xs'
          onClick={() => setAdvanced(true)}
        >
          {t('sites.form.apiEndpointsAdvanced')}
        </Button>
      </div>
    </div>
  )
}
