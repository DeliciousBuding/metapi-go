// metapi-go/features/models — model detail Sheet (side panel).
//
// Opens from the row "view details" action. Shows the model's full
// capability set (endpoint types + tags), per-account pricing matrix, and
// the live availability rows (site / username / latency / balance /
// downstream tokens). The sheet also offers a CTA into the model tester
// so an operator can probe a model straight from the marketplace.

import { useNavigate } from '@tanstack/react-router'
import {
  ArrowRight as ArrowRightIcon,
  FlaskConical as FlaskConicalIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { BrandIcon } from '@/assets/brand-icons/BrandIcon'
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

import type { ModelGroupPricing, ModelRow } from '../types'

type ModelDetailSheetProps = {
  model: ModelRow | null
  open: boolean
  onOpenChange: (open: boolean) => void
  onTest?: (model: ModelRow) => void
}

function DetailRow({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className='grid grid-cols-3 gap-2 py-1.5 text-sm'>
      <span className='text-muted-foreground col-span-1'>{label}</span>
      <div className='col-span-2 break-words'>{children}</div>
    </div>
  )
}

function formatPrice(value: number | undefined): string {
  if (value === undefined || value === null || !Number.isFinite(value)) {
    return '—'
  }
  if (value === 0) return '0'
  if (value < 0.01) return value.toFixed(4)
  return value.toFixed(2)
}

function formatLatency(latency: number | null): string {
  if (latency === null || latency === undefined) return '—'
  return `${Math.round(latency)}ms`
}

function formatSuccessRate(rate: number | null): string {
  if (rate === null || rate === undefined) return '—'
  return `${Math.round(rate * 100)}%`
}

function GroupPricingRow({
  groupKey,
  pricing,
}: {
  groupKey: string
  pricing: ModelGroupPricing
}) {
  return (
    <li className='flex flex-wrap items-center gap-2 text-xs'>
      <Badge variant='outline'>{groupKey}</Badge>
      <span className='text-muted-foreground'>
        in ${formatPrice(pricing.inputPerMillion)}/M
      </span>
      <span className='text-muted-foreground'>
        out ${formatPrice(pricing.outputPerMillion)}/M
      </span>
    </li>
  )
}

export function ModelDetailSheet({
  model,
  open,
  onOpenChange,
  onTest,
}: ModelDetailSheetProps) {
  const { t } = useTranslation()
  const navigate = useNavigate()

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

  function goToTester() {
    onOpenChange(false)
    const params = new URLSearchParams({ model: model?.name ?? '' })
    navigate({ href: `/model-tester?${params.toString()}`, replace: true })
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent side='right' className='sm:max-w-md'>
        <SheetHeader>
          <div className='flex items-center gap-3 pr-6'>
            <BrandIcon model={model.name} size={40} />
            <div className='min-w-0 flex-1'>
              <SheetTitle className='truncate'>{model.name}</SheetTitle>
              <SheetDescription className='truncate'>
                {model.description || t('models.detail.noDescription')}
              </SheetDescription>
            </div>
          </div>
        </SheetHeader>

        <ScrollArea className='flex-1'>
          <div className='flex flex-col gap-4 px-4 pb-4'>
            <section className='grid grid-cols-2 gap-2 text-sm sm:grid-cols-4'>
              <DetailRow label={t('models.detail.accounts')}>
                {model.accountCount}
              </DetailRow>
              <DetailRow label={t('models.detail.tokens')}>
                {model.tokenCount}
              </DetailRow>
              <DetailRow label={t('models.detail.latency')}>
                {formatLatency(model.avgLatency)}
              </DetailRow>
              <DetailRow label={t('models.detail.successRate')}>
                {formatSuccessRate(model.successRate)}
              </DetailRow>
            </section>

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
                            <Badge variant='default'>{source.siteName}</Badge>
                            <span className='text-muted-foreground'>
                              {source.username ?? t('models.detail.unknownUser')}
                            </span>
                          </div>
                          {groupKeys.length > 0 && (
                            <ul className='ml-2 flex flex-col gap-1 border-l pl-3'>
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
                          ${formatPrice(account.balance)}
                        </span>
                      </li>
                    ))}
                  </ul>
                </section>
              </>
            )}

            <Separator />

            <section className='flex flex-col gap-2'>
              {onTest && (
                <Button variant='default' onClick={() => onTest(model)}>
                  <FlaskConicalIcon className='mr-1 size-4' />
                  {t('models.detail.testModel')}
                </Button>
              )}
              <Button variant='outline' onClick={goToTester}>
                {t('models.detail.openTester')}
                <ArrowRightIcon className='ml-1 size-4' />
              </Button>
            </section>
          </div>
        </ScrollArea>
      </SheetContent>
    </Sheet>
  )
}
