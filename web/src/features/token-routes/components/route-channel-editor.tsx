// metapi-go features/token-routes/components — per-channel editor embedded in
// the route edit dialog (edit mode only).
//
// Weight / priority / enabled / delete map 1:1 to PUT/DELETE /api/channels/{id};
// every edit sets manual_override on the backend so a route rebuild cannot
// wipe operator tuning. Edits commit immediately (inline-save semantics, the
// pattern of the TS original's per-channel inline save) — the React Hook Form
// state in the owning dialog only covers route-level fields, so channel
// changes are never staged behind the route Save button.

import { Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'

import { useDeleteChannel, useRouteChannels, useUpdateChannel } from '../api'
import type { RouteChannel } from '../types'

const GRID_CLASS =
  'grid grid-cols-[minmax(0,1fr)_64px_64px_40px_40px] items-center gap-2'

type NumberFieldProps = {
  value: number | null | undefined
  label: string
  onCommit: (next: number) => void
  disabled?: boolean
  /** Bumped by the caller when the mutation for this field fails, so a
   * user-typed value the backend rejected reverts to the last fetched one. */
  revertTick?: number
}

/**
 * Integer-only mini input that commits on blur / Enter and reverts on an
 * unparsable or negative value. Controlled by the channel's fetched value so
 * a failed mutation (or a stale row) re-reverses itself on the next refetch.
 */
function NumberField({
  value,
  label,
  onCommit,
  disabled,
  revertTick = 0,
}: NumberFieldProps) {
  const [draft, setDraft] = useState(String(value ?? ''))
  useEffect(() => {
    setDraft(String(value ?? ''))
  }, [value, revertTick])

  const commit = () => {
    const trimmed = draft.trim()
    if (trimmed === '') {
      setDraft(String(value ?? ''))
      return
    }
    const parsed = Number.parseInt(trimmed, 10)
    if (String(parsed) !== trimmed || !Number.isInteger(parsed) || parsed < 0) {
      // Non-integer input (e.g. "1.5" or "abc") — revert instead of sending
      // a value the backend would coerce unpredictably.
      setDraft(String(value ?? ''))
      return
    }
    if (parsed !== value) {
      onCommit(parsed)
    } else {
      setDraft(String(value ?? ''))
    }
  }

  return (
    <Input
      type='number'
      inputMode='numeric'
      min={0}
      step={1}
      className='h-8 w-16 px-2 text-sm tabular-nums'
      aria-label={label}
      value={draft}
      disabled={disabled}
      onChange={(event) => setDraft(event.target.value)}
      onBlur={commit}
      onKeyDown={(event) => {
        if (event.key === 'Enter') {
          event.preventDefault()
          event.currentTarget.blur()
        }
      }}
    />
  )
}

function ChannelEditRow({
  channel,
  disabled,
  pending,
}: {
  channel: RouteChannel
  disabled: boolean
  pending: boolean
}) {
  const { t } = useTranslation()
  const updateMutation = useUpdateChannel()
  const deleteMutation = useDeleteChannel()
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [revertTick, setRevertTick] = useState(0)

  const accountLabel =
    channel.account?.username ||
    t('tokenRoutes.detail.fallbackAccount', { id: channel.accountId })
  const secondLine = [
    channel.site?.name || channel.site?.platform ? `@ ${channel.site?.name || channel.site?.platform}` : null,
    channel.sourceModel || null,
    channel.token?.name
      ? `${t('tokenRoutes.formChannel.tokenLabel')}: ${channel.token.name}`
      : null,
  ]
    .filter(Boolean)
    .join(' · ')

  const rowBusy = disabled || pending

  const commitField = (data: { weight?: number; priority?: number }) => {
    updateMutation.mutate(
      { id: channel.id, data },
      {
        onError: () =>
          // Bump so the rejected draft reverts to the fetched value.
          setRevertTick((tick) => tick + 1),
      }
    )
  }

  return (
    <li className={`${GRID_CLASS} py-1.5`}>
      <div className='min-w-0 pr-1'>
        <div className='truncate text-sm font-medium' title={accountLabel}>
          {accountLabel}
        </div>
        {secondLine && (
          <div className='text-muted-foreground truncate text-[11px]' title={secondLine}>
            {secondLine}
          </div>
        )}
      </div>

      <NumberField
        value={channel.weight}
        label={t('tokenRoutes.formChannel.weightLabel')}
        disabled={rowBusy}
        revertTick={revertTick}
        onCommit={(weight) => commitField({ weight })}
      />
      <NumberField
        value={channel.priority}
        label={t('tokenRoutes.formChannel.priorityLabel')}
        disabled={rowBusy}
        revertTick={revertTick}
        onCommit={(priority) => commitField({ priority })}
      />

      <div className='flex justify-center'>
        {pending && updateMutation.variables?.id === channel.id ? (
          <Spinner className='size-4' />
        ) : (
          <Switch
            checked={channel.enabled}
            aria-label={t('tokenRoutes.formChannel.enabledLabel')}
            disabled={rowBusy}
            onCheckedChange={() =>
              updateMutation.mutate({
                id: channel.id,
                data: { enabled: !channel.enabled },
              })
            }
          />
        )}
      </div>

      {pending && deleteMutation.variables === channel.id ? (
        <Spinner className='justify-self-center size-4' />
      ) : (
        <Button
          type='button'
          variant='ghost'
          size='icon-sm'
          className='text-muted-foreground hover:text-destructive justify-self-center'
          aria-label={t('tokenRoutes.formChannel.removeLabel')}
          disabled={rowBusy}
          onClick={() => setConfirmOpen(true)}
        >
          <Trash2 className='size-4' />
        </Button>
      )}

      <ConfirmDialog
        open={confirmOpen}
        title={t('tokenRoutes.formChannel.removeTitle')}
        description={t('tokenRoutes.formChannel.removeDescription', {
          name: accountLabel,
        })}
        confirmLabel={t('tokenRoutes.formChannel.removeConfirm')}
        cancelLabel={t('common.cancel')}
        destructive
        onConfirm={() => {
          setConfirmOpen(false)
          deleteMutation.mutate(channel.id)
        }}
        onCancel={() => setConfirmOpen(false)}
      />
    </li>
  )
}

export function RouteChannelEditor({ routeId }: { routeId: number }) {
  const { t } = useTranslation()
  const channelsQuery = useRouteChannels(routeId)
  const channels = channelsQuery.data ?? []
  const busy = channelsQuery.isLoading || channelsQuery.isFetching

  return (
    <section className='space-y-2'>
      <div className='flex items-center justify-between'>
        <div className='space-y-0.5'>
          <h4 className='text-sm font-medium'>
            {t('tokenRoutes.formChannel.title')}
          </h4>
          <p className='text-muted-foreground text-xs'>
            {t('tokenRoutes.formChannel.hint')}
          </p>
        </div>
        {channelsQuery.isFetching && (
          <Spinner className='text-muted-foreground size-3.5' />
        )}
      </div>

      {channelsQuery.isLoading ? (
        <div className='text-muted-foreground flex items-center gap-2 rounded-lg border p-3 text-sm'>
          <Spinner className='size-4' />
          {t('tokenRoutes.formChannel.loading')}
        </div>
      ) : channels.length === 0 ? (
        <p className='text-muted-foreground rounded-lg border border-dashed p-3 text-sm'>
          {t('tokenRoutes.formChannel.empty')}
        </p>
      ) : (
        <div className='rounded-lg border'>
          <div
            className={`${GRID_CLASS} text-muted-foreground border-b px-2 py-1 text-[11px]`}
          >
            <span />
            <span>{t('tokenRoutes.formChannel.weightLabel')}</span>
            <span>{t('tokenRoutes.formChannel.priorityLabel')}</span>
            <span className='justify-self-center'>
              {t('tokenRoutes.formChannel.enabledLabel')}
            </span>
            <span />
          </div>
          <ul className='divide-y px-2'>
            {channels.map((channel) => (
              <ChannelEditRow
                key={channel.id}
                channel={channel}
                disabled={busy}
                pending={busy}
              />
            ))}
          </ul>
        </div>
      )}
    </section>
  )
}
