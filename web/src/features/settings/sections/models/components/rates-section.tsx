// metapi-go/features/settings/sections/models/components — rates overview
// section (N9a). Read-only aggregation of account unit-cost + channel weight
// with inline ✎ edit (Enter=commit, Esc=cancel). Mirrors the legacy
// RatesOverviewSection but trimmed to the two editable surfaces.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { api,type RateOverviewResponse } from '@/lib/api'

import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import {
  SettingsSectionCard,
  SettingsSectionSkeleton,
} from '../../../components/settings-section-card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

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

  useEffect(() => {
    // Cancel the in-flight edit if the underlying data refreshes.
    setDraft(null)
  }, [overviewQuery.data])

  const updateMutation = useMutation({
    mutationFn: async (payload: {
      accounts?: Array<{ id: number; unitCost: number }>
      channels?: Array<{ id: number; weight: number }>
    }) => api.updateRates(payload),
    onSuccess: (result) => {
      void queryClient.invalidateQueries({ queryKey: ratesQueryKeys.all })
      setDraft(null)
      toast.success(
        t('settings.models.rates.toast.updated', {
          accounts: result.updatedAccounts,
          channels: result.updatedChannels,
        }),
      )
    },
    onError: () => toast.error(t('settings.models.rates.toast.updateFailed')),
  })

  function commitEdit(target: EditTarget) {
    if (!target) {
      return
    }
    if (target.kind === 'account') {
      updateMutation.mutate({ accounts: [{ id: target.id, unitCost: target.value }] })
    } else {
      updateMutation.mutate({ channels: [{ id: target.id, weight: target.value }] })
    }
  }

  if (overviewQuery.isLoading) {
    return <SettingsSectionSkeleton />
  }

  const overview = overviewQuery.data
  if (!overview) {
    return (
      <SettingsSectionCard
        title={t('settings.models.rates.title')}
        description={t('settings.models.rates.description')}
      >
        <p className='py-8 text-center text-sm text-muted-foreground'>
          {t('settings.models.rates.empty')}
        </p>
      </SettingsSectionCard>
    )
  }

  const summary = overview.summary

  return (
    <SettingsSectionCard
      title={t('settings.models.rates.title')}
      description={t('settings.models.rates.description')}
    >
      <p className='mb-4 text-xs text-muted-foreground'>
        {t('settings.models.rates.summary', {
          accounts: summary.accountsTotal,
          withCost: summary.accountsWithUnitCost,
          channels: summary.channelsTotal,
          channelsEnabled: summary.channelsEnabled,
        })}
      </p>

      <h4 className='mb-2 text-sm font-medium'>
        {t('settings.models.rates.accountTable')}
      </h4>
      <Table className='mb-6'>
        <TableHeader>
          <TableRow>
            <TableHead>{t('settings.models.rates.columns.account')}</TableHead>
            <TableHead>{t('settings.models.rates.columns.site')}</TableHead>
            <TableHead>{t('settings.models.rates.columns.channels')}</TableHead>
            <TableHead>{t('settings.models.rates.columns.unitCost')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {overview.accounts.map((account) => (
            <TableRow key={account.accountId}>
              <TableCell className='text-sm'>{account.username}</TableCell>
              <TableCell className='text-xs text-muted-foreground'>
                {account.siteName}
              </TableCell>
              <TableCell>{account.channelCount}</TableCell>
              <TableCell>
                {draft?.kind === 'account' && draft.id === account.accountId ? (
                  <Input
                    type='number'
                    min={0}
                    step='any'
                    autoFocus
                    defaultValue={account.unitCost ?? 0}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter') {
                        const value = Number((event.target as HTMLInputElement).value)
                        commitEdit({ kind: 'account', id: account.accountId, value })
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
                    <span className='text-xs text-muted-foreground'>✎</span>
                  </button>
                )}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>

      <h4 className='mb-2 text-sm font-medium'>
        {t('settings.models.rates.channelTable')}
      </h4>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t('settings.models.rates.columns.route')}</TableHead>
            <TableHead>{t('settings.models.rates.columns.model')}</TableHead>
            <TableHead>{t('settings.models.rates.columns.account')}</TableHead>
            <TableHead>{t('settings.models.rates.columns.enabled')}</TableHead>
            <TableHead>{t('settings.models.rates.columns.weight')}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {overview.channels.map((channel) => (
            <TableRow key={channel.channelId}>
              <TableCell className='font-mono text-xs'>{channel.routePattern}</TableCell>
              <TableCell className='font-mono text-xs'>{channel.modelName}</TableCell>
              <TableCell className='text-xs'>{channel.username}</TableCell>
              <TableCell>
                <Badge variant={channel.enabled ? 'default' : 'secondary'}>
                  {channel.enabled
                    ? t('settings.common.enabled')
                    : t('settings.common.disabled')}
                </Badge>
              </TableCell>
              <TableCell>
                {draft?.kind === 'channel' && draft.id === channel.channelId ? (
                  <Input
                    type='number'
                    min={0}
                    step='any'
                    autoFocus
                    defaultValue={channel.weight}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter') {
                        const value = Number((event.target as HTMLInputElement).value)
                        commitEdit({ kind: 'channel', id: channel.channelId, value })
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
                    <span className='text-xs text-muted-foreground'>✎</span>
                  </button>
                )}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </SettingsSectionCard>
  )
}
