// metapi-go/features/settings/sections/proxy-models/components — rates overview
// section (N9a). Account unit-cost + channel weight aggregation with inline
// ✎ edit (Enter=commit, Esc=cancel). Mirrors the legacy
// RatesOverviewSection but trimmed to the two editable surfaces.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { api, type RateOverviewResponse } from '@/lib/api'
import { toast } from '@/lib/toast'

import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'
import { SettingsSubsection } from '../../../components/settings-subsection'

const ratesQueryKeys = {
  all: ['model-rates'] as const,
  overview: () => [...ratesQueryKeys.all, 'overview'] as const,
}

type EditTarget =
  | { kind: 'account'; id: number; value: number }
  | { kind: 'channel'; id: number; value: number }
  | null

export function RatesSection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const overviewQuery = useQuery<RateOverviewResponse>({
    queryKey: ratesQueryKeys.overview(),
    queryFn: async () => api.getRateOverview(),
    staleTime: 30 * 1000,
  })

  const [draft, setDraft] = useState<EditTarget>(null)

  // Note: the draft is intentionally NOT cleared when overviewQuery.data
  // refreshes — a background refetch must not silently discard the value an
  // operator is typing. The input renders the draft state, so a refetch only
  // re-renders the non-editing rows.

  const updateMutation = useMutation({
    mutationFn: async (payload: {
      accounts?: Array<{ id: number; unitCost: number }>
      channels?: Array<{ id: number; weight: number }>
    }) => api.updateRates(payload),
    onSuccess: (result) => {
      void queryClient.invalidateQueries({ queryKey: ratesQueryKeys.all })
      setDraft(null)
      toast.success(
        t('settings.proxyModels.rates.toast.updated', {
          accounts: result.updatedAccounts,
          channels: result.updatedChannels,
        })
      )
    },
    onError: () =>
      toast.error(t('settings.proxyModels.rates.toast.updateFailed')),
  })

  function commitEdit(target: EditTarget) {
    if (!target) {
      return
    }
    if (updateMutation.isPending) {
      // In-flight edit already — ignore the extra Enter keystroke.
      return
    }
    if (typeof target.value !== 'number' || Number.isNaN(target.value)) {
      // Empty / non-numeric input must never silently become 0.
      toast.error(t('settings.proxyModels.rates.toast.invalidValue'))
      return
    }
    if (target.kind === 'account') {
      updateMutation.mutate({
        accounts: [{ id: target.id, unitCost: target.value }],
      })
    } else {
      updateMutation.mutate({
        channels: [{ id: target.id, weight: target.value }],
      })
    }
  }

  if (overviewQuery.isLoading) {
    return <SettingsSectionSkeleton />
  }

  const overview = overviewQuery.data
  if (!overview) {
    return (
      <SettingsSectionCard
        title={t('settings.proxyModels.rates.title')}
        description={t('settings.proxyModels.rates.description')}
      >
        <p className='text-muted-foreground py-8 text-center text-sm'>
          {t('settings.proxyModels.rates.empty')}
        </p>
      </SettingsSectionCard>
    )
  }

  const summary = overview.summary

  return (
    <SettingsSectionCard
      title={t('settings.proxyModels.rates.title')}
      description={t('settings.proxyModels.rates.description')}
    >
      <p className='text-muted-foreground mb-4 text-xs'>
        {t('settings.proxyModels.rates.summary', {
          accounts: summary.accountsTotal,
          withCost: summary.accountsWithUnitCost,
          channels: summary.channelsTotal,
          channelsEnabled: summary.channelsEnabled,
        })}
      </p>

      <SettingsSubsection title={t('settings.proxyModels.rates.accountTable')}>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>
                {t('settings.proxyModels.rates.columns.account')}
              </TableHead>
              <TableHead>
                {t('settings.proxyModels.rates.columns.site')}
              </TableHead>
              <TableHead>
                {t('settings.proxyModels.rates.columns.channels')}
              </TableHead>
              <TableHead>
                {t('settings.proxyModels.rates.columns.unitCost')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {overview.accounts.map((account) => (
              <TableRow key={account.accountId}>
                <TableCell className='text-sm'>{account.username}</TableCell>
                <TableCell className='text-muted-foreground text-xs'>
                  {account.siteName}
                </TableCell>
                <TableCell>{account.channelCount}</TableCell>
                <TableCell>
                  {draft?.kind === 'account' &&
                  draft.id === account.accountId ? (
                    <Input
                      type='number'
                      min={0}
                      step='any'
                      autoFocus
                      disabled={updateMutation.isPending}
                      defaultValue={account.unitCost ?? 0}
                      onKeyDown={(event) => {
                        if (event.key === 'Enter') {
                          const value = Number(
                            (event.target as HTMLInputElement).value
                          )
                          commitEdit({
                            kind: 'account',
                            id: account.accountId,
                            value,
                          })
                        }
                        if (event.key === 'Escape') {
                          setDraft(null)
                        }
                      }}
                      className='h-7 w-24'
                    />
                  ) : (
                    <button
                      type='button'
                      aria-label={t('settings.proxyModels.rates.editUnitCost', {
                        account: account.username,
                      })}
                      className='flex items-center gap-1 text-sm'
                      onClick={() =>
                        setDraft({
                          kind: 'account',
                          id: account.accountId,
                          value: account.unitCost ?? 0,
                        })
                      }
                    >
                      {account.unitCost ?? '—'}
                      <span className='text-muted-foreground text-xs'>✎</span>
                    </button>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </SettingsSubsection>

      <SettingsSubsection title={t('settings.proxyModels.rates.channelTable')}>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>
                {t('settings.proxyModels.rates.columns.route')}
              </TableHead>
              <TableHead>
                {t('settings.proxyModels.rates.columns.model')}
              </TableHead>
              <TableHead>
                {t('settings.proxyModels.rates.columns.account')}
              </TableHead>
              <TableHead>
                {t('settings.proxyModels.rates.columns.enabled')}
              </TableHead>
              <TableHead>
                {t('settings.proxyModels.rates.columns.weight')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {overview.channels.map((channel) => (
              <TableRow key={channel.channelId}>
                <TableCell className='font-mono text-xs'>
                  {channel.routePattern}
                </TableCell>
                <TableCell className='font-mono text-xs'>
                  {channel.modelName}
                </TableCell>
                <TableCell className='text-xs'>{channel.username}</TableCell>
                <TableCell>
                  <Badge variant={channel.enabled ? 'default' : 'secondary'}>
                    {channel.enabled
                      ? t('settings.common.enabled')
                      : t('settings.common.disabled')}
                  </Badge>
                </TableCell>
                <TableCell>
                  {draft?.kind === 'channel' &&
                  draft.id === channel.channelId ? (
                    <Input
                      type='number'
                      min={0}
                      step='any'
                      autoFocus
                      disabled={updateMutation.isPending}
                      defaultValue={channel.weight}
                      onKeyDown={(event) => {
                        if (event.key === 'Enter') {
                          const value = Number(
                            (event.target as HTMLInputElement).value
                          )
                          commitEdit({
                            kind: 'channel',
                            id: channel.channelId,
                            value,
                          })
                        }
                        if (event.key === 'Escape') {
                          setDraft(null)
                        }
                      }}
                      className='h-7 w-24'
                    />
                  ) : (
                    <button
                      type='button'
                      aria-label={t('settings.proxyModels.rates.editWeight', {
                        model: channel.modelName,
                      })}
                      className='flex items-center gap-1 text-sm'
                      onClick={() =>
                        setDraft({
                          kind: 'channel',
                          id: channel.channelId,
                          value: channel.weight,
                        })
                      }
                    >
                      {channel.weight}
                      <span className='text-muted-foreground text-xs'>✎</span>
                    </button>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </SettingsSubsection>
    </SettingsSectionCard>
  )
}
