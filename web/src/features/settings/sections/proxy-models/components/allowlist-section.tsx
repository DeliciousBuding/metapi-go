// metapi-go/features/settings/sections/proxy-models/components — global allowlist
// + brand-blocking section (legacy cards 10-11). globalAllowedModels is an
// inline text input + badges; globalBlockedBrands is a grid of toggle
// switches sourced from api.getBrandList(). Both saves trigger a routes
// rebuild through the shared task observer so completion is not inferred from
// a launch acknowledgement.

import { useTranslation } from 'react-i18next'

import { SectionCard } from '@/components/common/section-card'
import { SectionError } from '@/components/common/section-error'
import { Form } from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'

import { FormNavigationGuard } from '../../../components/form-navigation-guard'
import { SettingsFormActions } from '../../../components/settings-form-actions'
import { AllowedModelsField } from './allowed-models-field'
import { useModelAllowlist } from './use-model-allowlist'

const ALLOWLIST_FORM_ID = 'settings-proxy-models-allowlist-form'

export function AllowlistSection() {
  const { t } = useTranslation()
  const {
    data,
    isLoading,
    isError,
    refetch,
    form,
    onSubmit,
    reset,
    isSaving,
    models,
    brands,
    loadingBrands,
    blockedBrands,
    toggleBrand,
    brandTogglePending,
  } = useModelAllowlist()
  if (isLoading) {
    return (
      <SectionCard
        title={t('settings.proxyModels.allowlist.title')}
        description={t('settings.proxyModels.allowlist.description')}
      >
        <p className='text-muted-foreground text-sm'>
          {t('settings.common.loading')}
        </p>
      </SectionCard>
    )
  }

  if (isError || !data) {
    return (
      <SectionError
        title={t('settings.proxyModels.allowlist.title')}
        messageKey='settings.common.loadFailed'
        onRetry={() => void refetch()}
      />
    )
  }

  const isDirty = form.formState.isDirty

  return (
    <SectionCard
      title={t('settings.proxyModels.allowlist.title')}
      description={t('settings.proxyModels.allowlist.description')}
    >
      <div className='space-y-4'>
        <Form {...form}>
          <form
            id={ALLOWLIST_FORM_ID}
            onSubmit={form.handleSubmit(onSubmit)}
            className='space-y-4'
          >
            <AllowedModelsField control={form.control} models={models} />
            <SettingsFormActions
              formId={ALLOWLIST_FORM_ID}
              isDirty={isDirty}
              isPending={isSaving}
              onReset={reset}
            />
          </form>
        </Form>

        <div className='space-y-3 rounded-lg border p-4'>
          <div>
            <h3 className='text-sm font-medium'>
              {t('settings.proxyModels.allowlist.fields.globalBlockedBrands')}
            </h3>
            <p className='text-muted-foreground text-xs'>
              {t(
                'settings.proxyModels.allowlist.fields.globalBlockedBrandsHint'
              )}
            </p>
          </div>
          {loadingBrands ? (
            <p className='text-muted-foreground text-sm'>
              {t('settings.proxyModels.allowlist.loadingBrands')}
            </p>
          ) : null}
          {!loadingBrands && brands.length === 0 ? (
            <p className='text-muted-foreground text-sm'>
              {t('settings.proxyModels.allowlist.noBrands')}
            </p>
          ) : null}
          {!loadingBrands && brands.length > 0 ? (
            <div className='grid grid-cols-2 gap-3 sm:grid-cols-3'>
              {brands.map((brand) => (
                <div
                  key={brand}
                  className='flex items-center justify-between gap-2 rounded-md border p-2'
                >
                  <span className='text-sm'>{brand}</span>
                  <Switch
                    checked={!blockedBrands.includes(brand)}
                    onCheckedChange={() => toggleBrand(brand)}
                    disabled={brandTogglePending}
                    aria-label={brand}
                  />
                </div>
              ))}
            </div>
          ) : null}
          {blockedBrands.length > 0 ? (
            <p className='text-muted-foreground text-xs'>
              {t('settings.proxyModels.allowlist.blockedCount', {
                count: blockedBrands.length,
              })}
            </p>
          ) : null}
        </div>
      </div>
      <FormNavigationGuard enabled={isDirty} />
    </SectionCard>
  )
}
