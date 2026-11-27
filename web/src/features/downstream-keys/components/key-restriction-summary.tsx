import { TriangleAlert } from 'lucide-react'
import { useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { detectPolicyIntersectionWarnings } from '../lib/key-policy-presets'
import {
  locateKeyScopeField,
  type CreateKeyFormValues,
} from './key-form-shared'

// Quick navigation reads the same form values; it never evaluates permissions.
export function KeyRestrictionSummary({
  routeGrantCount = 0,
}: {
  routeGrantCount?: number
}) {
  const { t } = useTranslation()
  const form = useFormContext<CreateKeyFormValues>()
  const values = useWatch({ control: form.control })
  const policy = values.accessPolicy
  let accessDimensionCount: number | null = 0
  if (policy === undefined) {
    accessDimensionCount = null
  } else if (policy) {
    accessDimensionCount = Object.keys(policy).filter(
      (key) => policy[key as keyof typeof policy] !== undefined
    ).length
  }
  const warnings = detectPolicyIntersectionWarnings({
    supportedModels: values.supportedModels,
    routeGrantCount,
    allowedCredentialRefs: values.allowedCredentialRefs,
    excludedCredentialRefs: values.excludedCredentialRefs,
    accessPolicy: policy,
    maxRequests: values.maxRequests,
    maxCost: values.maxCost,
  })
  const dimensions = [
    {
      field: 'supportedModels',
      label: t('settings.downstream.keys.models.inputAria'),
      count: values.supportedModels?.length ?? 0,
    },
    {
      field: 'allowedSiteIds',
      label: t('settings.downstream.keys.scope.sitesAllow'),
      count: values.allowedSiteIds?.length ?? 0,
    },
    {
      field: 'allowedCredentialRefs',
      label: t('settings.downstream.keys.scope.credsAllow'),
      count: values.allowedCredentialRefs?.length ?? 0,
    },
    {
      field: 'excludedCredentialRefs',
      label: t('settings.downstream.keys.scope.credsExclude'),
      count: values.excludedCredentialRefs?.length ?? 0,
    },
    {
      field: 'accessPolicy',
      label: t('settings.downstream.keys.fields.accessPolicy'),
      count: accessDimensionCount,
    },
  ]
  return (
    <div className='bg-background sticky top-0 z-10 space-y-2 border-b pt-1 pb-2'>
      <nav
        className='flex flex-wrap gap-1'
        aria-label={t('settings.downstream.keys.columns.scope')}
      >
        {dimensions.map((dimension) => (
          <Button
            key={dimension.field}
            type='button'
            variant={
              dimension.count !== null && dimension.count > 0
                ? 'secondary'
                : 'outline'
            }
            size='xs'
            onClick={() => locateKeyScopeField(dimension.field)}
          >
            {dimension.label} ·{' '}
            {dimension.count ?? t('settings.downstream.keys.picker.unknown')}
          </Button>
        ))}
      </nav>
      {warnings.length > 0 && (
        <div
          role='status'
          data-testid='key-policy-intersection-warnings'
          className='border-warning/40 bg-warning/10 space-y-1 rounded-md border px-2.5 py-2 text-xs'
        >
          <p className='text-warning-soft-fg flex items-center gap-1.5 font-medium'>
            <TriangleAlert className='size-3.5 shrink-0' aria-hidden='true' />
            {t('settings.downstream.keys.warnings.title')}
          </p>
          <ul className='text-muted-foreground list-disc space-y-0.5 pl-5'>
            {warnings.map((code) => (
              <li key={code}>
                {t(`settings.downstream.keys.warnings.${code}`)}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
