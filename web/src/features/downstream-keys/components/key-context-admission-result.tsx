import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import type { KeyContextDecisionResponse } from '@/lib/api'

/**
 * Server-verdict renderer for a downstream key's admission chain. The route
 * workspace and the key dialog both ask “can this key call this model?”;
 * duplicating the verdict UI would let allow/deny semantics drift.
 */
export function KeyContextAdmissionResult(props: {
  result: KeyContextDecisionResponse
}) {
  const { t } = useTranslation()
  const admission = props.result.keyContext.admission
  return (
    <div className='flex flex-col gap-3 border-t pt-3'>
      <div className='flex items-center gap-2'>
        {admission.verdict === 'allow' ? (
          <Badge variant='default'>
            {t('settings.downstream.keys.check.allow')}
          </Badge>
        ) : (
          <Badge variant='destructive'>
            {t('settings.downstream.keys.check.deny')}
          </Badge>
        )}
        <span className='text-muted-foreground text-xs'>
          {t('settings.downstream.keys.check.readOnlyNote')}
        </span>
      </div>

      {admission.blockers.length > 0 ? (
        <ul className='flex flex-col gap-1.5'>
          {admission.blockers.map((blocker) => (
            <li
              key={`${blocker.dimension}-${blocker.code}`}
              className='flex items-start gap-2 text-sm'
            >
              <Badge variant='outline' className='shrink-0'>
                {t(
                  `settings.downstream.keys.check.dimensions.${blocker.dimension}`
                )}
              </Badge>
              <span className='min-w-0'>{blocker.message}</span>
            </li>
          ))}
        </ul>
      ) : null}

      <AdmissionWindows admission={admission} />
    </div>
  )
}

function AdmissionWindows({
  admission,
}: {
  admission: KeyContextDecisionResponse['keyContext']['admission']
}) {
  const { t } = useTranslation()
  const rows: Array<{ label: string; value: string }> = []
  const push = (
    label: string,
    used: number,
    max: number | null | undefined
  ) => {
    rows.push({
      label,
      value: max == null ? String(used) : `${used} / ${max}`,
    })
  }
  if (admission.cost) {
    push(
      t('settings.downstream.keys.check.cost'),
      admission.cost.used,
      admission.cost.max
    )
  }
  if (admission.requests) {
    push(
      t('settings.downstream.keys.check.requests'),
      admission.requests.used,
      admission.requests.max
    )
  }
  if (admission.rate) {
    push(
      t('settings.downstream.keys.check.rpm'),
      admission.rate.rpm.used,
      admission.rate.rpm.max
    )
    push(
      t('settings.downstream.keys.check.tpm'),
      admission.rate.tpm.used,
      admission.rate.tpm.max
    )
  }
  if (admission.quota) {
    if (admission.quota.requestsMax != null) {
      push(
        t('settings.downstream.keys.check.quotaRequests'),
        admission.quota.requestsUsed,
        admission.quota.requestsMax
      )
    }
    if (admission.quota.tokensMax != null) {
      push(
        t('settings.downstream.keys.check.quotaTokens'),
        admission.quota.tokensUsed,
        admission.quota.tokensMax
      )
    }
    if (admission.quota.costMax != null) {
      push(
        t('settings.downstream.keys.check.quotaCost'),
        admission.quota.costUsed,
        admission.quota.costMax
      )
    }
  }
  if (rows.length === 0) return null
  return (
    <div className='flex flex-wrap gap-x-4 gap-y-1 text-xs tabular-nums'>
      {rows.map((row) => (
        <span key={row.label}>
          <span className='text-muted-foreground'>{row.label}</span> {row.value}
        </span>
      ))}
      {admission.rate ? (
        <span className='text-muted-foreground'>
          {t('settings.downstream.keys.check.rateScopeNote')}
        </span>
      ) : null}
    </div>
  )
}
