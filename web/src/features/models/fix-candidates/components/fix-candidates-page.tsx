// metapi-go/features/models/fix-candidates — list disabled models that an
// existing redirect can restore, with a one-click apply + result feedback.

import { CheckCircle2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
} from '@/components/ui/empty'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

import { useApplyRedirectFixCandidates, useRedirectFixCandidates } from '../api'

export function FixCandidatesPage() {
  const { t } = useTranslation()
  const list = useRedirectFixCandidates()
  const apply = useApplyRedirectFixCandidates()
  const [confirmOpen, setConfirmOpen] = useState(false)

  const candidates = list.data?.items ?? []
  const count = list.data?.count ?? candidates.length

  return (
    <div className='flex h-full flex-col gap-3 p-4'>
      <div className='flex flex-wrap items-end justify-between gap-3'>
        <div>
          <h1 className='text-lg font-normal'>
            {t('fixCandidates.page.title')}
          </h1>
          <p className='text-muted-foreground text-sm'>
            {t('fixCandidates.page.description')}
          </p>
        </div>
        <Button
          size='sm'
          disabled={count === 0 || list.isLoading || apply.isPending}
          onClick={() => setConfirmOpen(true)}
        >
          <CheckCircle2 className='size-3.5' />
          {t('fixCandidates.apply')}
        </Button>
      </div>

      {list.isLoading && (
        <div className='text-muted-foreground flex items-center gap-2 text-sm'>
          <Spinner />
          {t('common.loading')}
        </div>
      )}

      <QueryErrorBanner
        error={list.error as Error | null}
        messageKey='fixCandidates.page.loadError'
        onRetry={() => list.refetch()}
        isRetrying={list.isFetching}
      />

      {!list.isLoading && !list.error && count === 0 && (
        <Empty className='border'>
          <EmptyHeader>
            <EmptyDescription>
              {t('fixCandidates.page.emptyDescription')}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}

      {!list.isLoading && !list.error && count > 0 && (
        <div className='overflow-x-auto rounded-lg border'>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('fixCandidates.columns.site')}</TableHead>
                <TableHead>{t('fixCandidates.columns.model')}</TableHead>
                <TableHead>{t('fixCandidates.columns.redirect')}</TableHead>
                <TableHead>{t('fixCandidates.columns.account')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {candidates.map((candidate) => (
                <TableRow
                  key={`${candidate.siteId}-${candidate.accountId}-${candidate.modelName}`}
                >
                  <TableCell className='font-medium'>
                    {candidate.siteName}
                  </TableCell>
                  <TableCell className='font-mono'>
                    {candidate.modelName}
                  </TableCell>
                  <TableCell className='text-muted-foreground font-mono'>
                    {candidate.canonical} → {candidate.actual}
                  </TableCell>
                  <TableCell className='tabular-nums'>
                    #{candidate.accountId}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <ConfirmDialog
        open={confirmOpen}
        title={t('fixCandidates.confirm.title')}
        description={t('fixCandidates.confirm.description', { count })}
        confirmLabel={t('common.confirm')}
        cancelLabel={t('common.cancel')}
        destructive
        onConfirm={() => {
          setConfirmOpen(false)
          apply.mutate()
        }}
        onCancel={() => setConfirmOpen(false)}
      />
    </div>
  )
}
