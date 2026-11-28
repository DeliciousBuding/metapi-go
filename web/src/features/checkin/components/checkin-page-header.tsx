import axios from 'axios'
import { RefreshCw, RotateCw, Zap } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { PageHeader } from '@/components/common/page-header'
import { QuietRefreshStatus } from '@/components/common/quiet-refresh-status'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { useDelayedSpin } from '@/hooks/use-quiet-refresh'
import { toast } from '@/lib/toast'

import { useManualCheckin, type useCheckinLogs } from '../api'

export function CheckinPageHeader({
  query,
  accountsEmpty,
  onManualCheckin,
}: {
  query: ReturnType<typeof useCheckinLogs>
  accountsEmpty: boolean
  onManualCheckin: () => void
}) {
  const { dataUpdatedAt, error, isFetching, refetch } = query
  const { t } = useTranslation()
  const showRefreshSpin = useDelayedSpin(isFetching)
  const triggerAllMutation = useManualCheckin()
  const handleTriggerAll = async () => {
    try {
      const result = await triggerAllMutation.mutateAsync()
      const summary = result.summary
      if (summary) {
        const desc = t('checkin.toast.summary', {
          total: summary.total,
          success: summary.success,
          failed: summary.failed,
          skipped: summary.skipped,
        })
        if (summary.failed > 0) {
          toast.error(t('checkin.toast.partialFailed'), { description: desc })
        } else {
          toast.success(t('checkin.toast.complete'), { description: desc })
        }
      } else {
        toast.success(result.message || t('checkin.toast.complete'))
      }
    } catch (error) {
      // Transport failures (non-2xx) are already toasted by the http-client
      // error interceptor; envelope failures thrown by the parser are not, so
      // they get their own honest error toast instead of being swallowed.
      if (!axios.isAxiosError(error)) {
        toast.error(
          error instanceof Error && error.message
            ? error.message
            : t('checkin.toast.triggerFailed')
        )
      }
    }
  }
  return (
    <PageHeader
      title={t('checkin.page.title')}
      description={t('checkin.page.description')}
      actions={
        <>
          <QuietRefreshStatus
            updatedAt={dataUpdatedAt ?? 0}
            hasError={Boolean(error)}
            className='hidden sm:inline'
          />
          <Button
            variant='outline'
            size='icon-sm'
            onClick={() => void refetch()}
            disabled={isFetching}
            aria-label={t('proxyLogs.page.refresh')}
            data-hit-area
          >
            <RefreshCw
              aria-hidden='true'
              className={showRefreshSpin ? 'animate-spin' : undefined}
            />
          </Button>
          <Button
            variant='outline'
            onClick={onManualCheckin}
            disabled={accountsEmpty}
          >
            <Zap />
            {t('checkin.page.manualCheckin')}
          </Button>
          <Button
            onClick={handleTriggerAll}
            disabled={triggerAllMutation.isPending}
          >
            {triggerAllMutation.isPending ? <Spinner /> : <RotateCw />}
            {t('checkin.page.runAll')}
          </Button>
        </>
      }
    />
  )
}
