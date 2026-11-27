// metapi-go/features/models — model detail Sheet (side panel).
//
// Opens from the model name or row menu. Copy and tester actions sit by the
// identity; descriptions, capabilities, pricing and account data follow below.

import { useNavigate } from '@tanstack/react-router'
import {
  ArrowRight as ArrowRightIcon,
  Copy as CopyIcon,
  Route as RouteIcon,
} from 'lucide-react'
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { BrandIcon, getBrand } from '@/assets/brand-icons/BrandIcon'
import { DetailField } from '@/components/common/detail-field'
import { ExpandableText } from '@/components/common/expandable-text'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { copyText } from '@/lib/clipboard'
import {
  formatCurrency,
  formatLatency,
  formatPrice,
  formatSuccessRate,
} from '@/lib/format'
import { toast } from '@/lib/toast'

import type { ModelGroupPricing, ModelRow } from '../types'
import { ModelSupplySources } from './model-supply-sources'
import { pricingSiteLabel } from './models-columns'

type ModelDetailSheetProps = {
  model: ModelRow | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

function formatPricingUnit(value: number | null | undefined): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '—'
  return `${formatPrice(value)}/M`
}

function GroupPricingRow({
  groupKey,
  pricing,
}: {
  groupKey: string
  pricing: ModelGroupPricing
}) {
  const { t } = useTranslation()
  const hasCacheRead =
    typeof pricing.cacheReadPerMillion === 'number' &&
    Number.isFinite(pricing.cacheReadPerMillion)
  const hasCacheWrite =
    typeof pricing.cacheWritePerMillion === 'number' &&
    Number.isFinite(pricing.cacheWritePerMillion)

  return (
    <li className='bg-muted/30 flex flex-col gap-2 rounded-lg border px-2.5 py-2 text-xs'>
      <div className='flex items-center justify-between gap-2'>
        <Badge variant='outline' className='font-medium'>
          {groupKey}
        </Badge>
      </div>
      <dl className='grid grid-cols-2 gap-x-3 gap-y-1.5 sm:grid-cols-4'>
        <div>
          <dt className='text-muted-foreground text-2xs'>
            {t('priceCompare.current.input')}
          </dt>
          <dd className='text-foreground mt-0.5 font-medium tabular-nums'>
            {formatPricingUnit(pricing.inputPerMillion)}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-2xs'>
            {t('priceCompare.current.output')}
          </dt>
          <dd className='text-foreground mt-0.5 font-medium tabular-nums'>
            {formatPricingUnit(pricing.outputPerMillion)}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-2xs'>
            {t('priceCompare.current.cacheRead')}
          </dt>
          <dd
            className={
              hasCacheRead
                ? 'text-info-soft-fg mt-0.5 font-medium tabular-nums'
                : 'text-muted-foreground mt-0.5 tabular-nums'
            }
          >
            {formatPricingUnit(pricing.cacheReadPerMillion)}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-2xs'>
            {t('priceCompare.current.cacheWrite')}
          </dt>
          <dd
            className={
              hasCacheWrite
                ? 'text-warning-soft-fg mt-0.5 font-medium tabular-nums'
                : 'text-muted-foreground mt-0.5 tabular-nums'
            }
          >
            {formatPricingUnit(pricing.cacheWritePerMillion)}
          </dd>
        </div>
      </dl>
    </li>
  )
}

export function ModelDetailSheet({
  model,
  open,
  onOpenChange,
}: ModelDetailSheetProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const titleRef = useRef<HTMLHeadingElement>(null)

  if (!model) {
    return (
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent side='right' />
      </Sheet>
    )
  }

  const endpointTypes = model.supportedEndpointTypes ?? []
  const tags = model.tags ?? []
  const pricingSources = model.pricingSources ?? []
  const accounts = model.accounts ?? []
  const modelName = model.name
  const brand = getBrand(modelName)

  async function copyModelName() {
    if (await copyText(modelName)) {
      toast.success(t('common.copied'))
    } else {
      toast.error(t('common.copyFailed'))
    }
  }

  function goToTester() {
    onOpenChange(false)
    const params = new URLSearchParams({ model: modelName })
    navigate({ href: `/model-tester?${params.toString()}`, replace: true })
  }

  function goToRouteDiagnosis() {
    onOpenChange(false)
    const params = new URLSearchParams({ workspaceModel: modelName })
    navigate({ href: `/token-routes?${params.toString()}` })
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side='right'
        className='sm:max-w-md'
        initialFocus={titleRef}
      >
        <SheetHeader>
          <div className='flex items-start gap-3'>
            <BrandIcon model={model.name} size={40} />
            <div className='min-w-0 flex-1 space-y-1'>
              <SheetTitle ref={titleRef} tabIndex={-1} className='outline-none'>
                {model.name}
              </SheetTitle>
              <div className='flex flex-wrap items-center gap-1.5'>
                {brand && (
                  <Badge variant='secondary' className='text-2xs'>
                    {brand.name}
                  </Badge>
                )}
                {model.deprecated && (
                  <Badge variant='warning' className='text-2xs'>
                    {t('models.columns.deprecated')}
                  </Badge>
                )}
              </div>
            </div>
          </div>
          <div className='flex flex-wrap gap-2 pt-2'>
            <Button
              variant='outline'
              size='sm'
              data-touch-target
              onClick={copyModelName}
            >
              <CopyIcon className='size-4' />
              {t('models.detail.copyName')}
            </Button>
            <Button
              variant='outline'
              size='sm'
              data-touch-target
              onClick={goToRouteDiagnosis}
            >
              <RouteIcon className='size-4' />
              {t('tokenRoutes.workspace.title')}
            </Button>
            <Button size='sm' data-touch-target onClick={goToTester}>
              {t('models.detail.openTester')}
              <ArrowRightIcon className='size-4' />
            </Button>
          </div>
        </SheetHeader>

        <ScrollArea className='flex-1'>
          <div className='flex flex-col gap-4 px-4 pb-4'>
            <SheetDescription render={<div />}>
              <ExpandableText
                text={model.description || t('models.detail.noDescription')}
              />
            </SheetDescription>
            <dl className='grid grid-cols-2 gap-3 text-sm sm:grid-cols-4'>
              <DetailField label={t('models.supply.title')}>
                {model.supplyCount ?? '—'}
              </DetailField>
              <DetailField label={t('models.detail.accounts')}>
                {model.accountCount}
              </DetailField>
              <DetailField label={t('models.detail.latency')}>
                {formatLatency(model.avgLatency)}
              </DetailField>
              <DetailField label={t('models.detail.successRate')}>
                {formatSuccessRate(model.successRate)}
              </DetailField>
            </dl>

            <Separator />
            <ModelSupplySources sources={model.supplySources} />

            {(endpointTypes.length > 0 || tags.length > 0) && (
              <>
                <Separator />
                <section>
                  <h3 className='text-sm font-medium'>
                    {t('models.detail.capabilities')}
                  </h3>
                  <div className='mt-2 flex flex-wrap gap-1.5'>
                    {endpointTypes.map((type) => (
                      <Badge key={`ep-${type}`} variant='secondary'>
                        {type}
                      </Badge>
                    ))}
                    {tags.map((tag) => (
                      <Badge key={`tag-${tag}`} variant='outline'>
                        {tag}
                      </Badge>
                    ))}
                  </div>
                </section>
              </>
            )}

            {pricingSources.length > 0 && (
              <>
                <Separator />
                <section>
                  <h3 className='text-sm font-medium'>
                    {t('models.detail.pricing')}
                  </h3>
                  <ul className='mt-2 space-y-3'>
                    {pricingSources.map((source) => {
                      const groupKeys = Object.keys(source.groupPricing ?? {})
                      return (
                        <li
                          key={`${source.siteId}-${source.accountId}`}
                          className='flex flex-col gap-1.5'
                        >
                          <div className='flex flex-wrap items-center gap-2 text-xs'>
                            <Badge variant='default'>
                              {pricingSiteLabel(
                                source.siteName,
                                source.siteId,
                                t
                              )}
                            </Badge>
                            {source.siteId !== 0 ? (
                              <span className='text-muted-foreground'>
                                {source.username ??
                                  t('models.detail.unknownUser')}
                              </span>
                            ) : null}
                          </div>
                          {groupKeys.length > 0 && (
                            <ul className='flex flex-col gap-1.5'>
                              {groupKeys.map((groupKey) => (
                                <GroupPricingRow
                                  key={groupKey}
                                  groupKey={groupKey}
                                  pricing={source.groupPricing[groupKey]}
                                />
                              ))}
                            </ul>
                          )}
                        </li>
                      )
                    })}
                  </ul>
                </section>
              </>
            )}

            {accounts.length > 0 && (
              <>
                <Separator />
                <section>
                  <h3 className='text-sm font-medium'>
                    {t('models.detail.availableAccounts')}
                  </h3>
                  <ul className='mt-2 space-y-1.5'>
                    {accounts.map((account) => (
                      <li
                        key={account.id}
                        className='flex flex-wrap items-center gap-2 text-sm'
                      >
                        <Badge variant='outline'>{account.site}</Badge>
                        <span className='text-muted-foreground'>
                          {account.username ?? t('models.detail.unknownUser')}
                        </span>
                        <span className='text-muted-foreground tabular-nums'>
                          {formatLatency(account.latency)}
                        </span>
                        <span className='text-muted-foreground tabular-nums'>
                          {formatCurrency(account.balance)}
                        </span>
                      </li>
                    ))}
                  </ul>
                </section>
              </>
            )}
          </div>
        </ScrollArea>
      </SheetContent>
    </Sheet>
  )
}
