import { useNavigate } from '@tanstack/react-router'
import type { Table } from '@tanstack/react-table'
import { CalendarCheck, Users, Zap } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { useSites } from '@/features/sites'

import type { useCheckinLogs } from '../api'
import type { CheckinLogRow } from '../types'

export function CheckinLogTable({
  query,
  accountsEmpty,
  onManualCheckin,
  table,
  handleResetFilters,
}: {
  query: ReturnType<typeof useCheckinLogs>
  accountsEmpty: boolean
  onManualCheckin: () => void
  table: Table<CheckinLogRow>
  handleResetFilters: () => void
}) {
  const { isLoading, isFetching, error, refetch } = query
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { data: sitesData } = useSites()
  const siteOptions = useMemo(
    () =>
      (sitesData ?? []).map((site) => ({
        value: site.name,
        label: site.name,
      })),
    [sitesData]
  )
  return (
    <DataTablePage
      table={table}
      isLoading={isLoading}
      isFetching={isFetching}
      error={error as Error | null}
      errorMessageKey='checkin.page.loadError'
      onErrorRetry={() => refetch()}
      isErrorRetrying={isFetching}
      emptyIcon={<CalendarCheck className='size-6' />}
      emptyTitle={t('checkin.page.emptyTitle')}
      emptyDescription={t('checkin.page.emptyDescription')}
      emptyAction={
        // With accounts present the CTA reuses the header's manual
        // check-in flow (same dialog, same mutation) to generate logs;
        // only a LOADED-AND-EMPTY account library points to the accounts
        // page instead — while the snapshot is still fetching the CTA
        // stays manual check-in and picks up the accounts as they land
        // (otherwise it flashed "Manage accounts" then back).
        accountsEmpty ? (
          <Button
            variant='outline'
            onClick={() => void navigate({ to: '/accounts' })}
          >
            <Users className='size-4' />
            {t('checkin.page.manageAccounts')}
          </Button>
        ) : (
          <Button onClick={onManualCheckin}>
            <Zap className='size-4' />
            {t('checkin.page.manualCheckin')}
          </Button>
        )
      }
      skeletonKeyPrefix='checkin-skeleton'
      toolbarProps={{
        searchPlaceholder: t('checkin.page.searchPlaceholder'),
        searchDebounceMs: 300,
        filters: [
          {
            columnId: 'status',
            title: t('checkin.page.filterStatusTitle'),
            singleSelect: true,
            options: [
              {
                label: t('checkin.page.filterStatusSuccess'),
                value: 'success',
              },
              {
                label: t('checkin.page.filterStatusFailed'),
                value: 'failed',
              },
              {
                label: t('checkin.page.filterStatusSkipped'),
                value: 'skipped',
              },
            ],
          },
          {
            columnId: 'reason',
            title: t('checkin.page.filterReasonTitle'),
            options: [
              {
                label: t('checkin.page.filterReasonVerification'),
                value: 'verification',
              },
              {
                label: t('checkin.page.filterReasonAuth'),
                value: 'auth',
              },
              {
                label: t('checkin.page.filterReasonNetwork'),
                value: 'network',
              },
              {
                label: t('checkin.page.filterReasonSite'),
                value: 'site',
              },
              {
                label: t('checkin.page.filterReasonState'),
                value: 'state',
              },
              {
                label: t('checkin.page.filterReasonUnknown'),
                value: 'unknown',
              },
            ],
          },
          ...(siteOptions.length > 0
            ? [
                {
                  columnId: 'site',
                  title: t('checkin.page.filterSiteTitle'),
                  options: siteOptions,
                },
              ]
            : []),
        ],
        onReset: handleResetFilters,
      }}
    />
  )
}
