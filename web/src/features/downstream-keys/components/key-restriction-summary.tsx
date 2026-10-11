import { useFormContext, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'

import { CREATE_FORM_ID, type CreateKeyFormValues } from './key-form-shared'

// Quick navigation reads the same form values; it never evaluates permissions.
export function KeyRestrictionSummary() {
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
  function locate(field: string) {
    const section = document
      .querySelector(`#${CREATE_FORM_ID}`)
      ?.querySelector<HTMLElement>(`[data-key-scope="${field}"]`)
    const control = section?.querySelector<HTMLElement>(
      'input, button, [tabindex="0"]'
    )
    control?.scrollIntoView?.({ block: 'center' })
    control?.focus({ preventScroll: true })
  }
  return (
    <nav
      className='bg-background sticky top-0 z-10 flex flex-wrap gap-1 border-b pt-1 pb-2'
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
          onClick={() => locate(dimension.field)}
        >
          {dimension.label} ·{' '}
          {dimension.count ?? t('settings.downstream.keys.picker.unknown')}
        </Button>
      ))}
    </nav>
  )
}
