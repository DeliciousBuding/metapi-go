import { Link, useLocation, useNavigate } from '@tanstack/react-router'
import { ArrowRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Notice } from '@/components/ui/notice'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import type { PersonalModel } from '../api'
import {
  modelContextHref,
  readModelContext,
  type PersonalProtocol,
} from '../connection-context'
import { connectionExample } from '../connection-example'
import { CopyValue } from './portal-keys'

function ReferencePrice({ model }: { model: PersonalModel }) {
  const { t, i18n } = useTranslation()
  const price = model.referencePrice
  const number = new Intl.NumberFormat(i18n.language, {
    maximumFractionDigits: 8,
  })
  return (
    <div className='bg-muted/30 space-y-2.5 rounded-lg border p-3'>
      <h3 className='text-xs font-medium'>
        {t('userPortal.catalog.referencePrice')}
      </h3>
      {price ? (
        <dl className='grid grid-cols-2 gap-x-4 gap-y-2 text-sm sm:grid-cols-4'>
          {(['input', 'output', 'cacheRead', 'cacheWrite'] as const).map(
            (field) => (
              <div key={field} className='min-w-0'>
                <dt className='text-muted-foreground text-xs'>
                  {t('userPortal.catalog.' + field)}
                </dt>
                <dd className='mt-0.5 font-medium tabular-nums'>
                  {price[field] === null
                    ? t('userPortal.unknown')
                    : number.format(price[field])}
                </dd>
              </div>
            )
          )}
        </dl>
      ) : (
        <p className='text-muted-foreground text-sm'>
          {t('userPortal.catalog.priceUnknown')}
        </p>
      )}
      <p className='text-muted-foreground text-xs leading-relaxed'>
        {t('userPortal.catalog.priceHint')}
      </p>
      {price?.updatedAt && (
        <p className='text-muted-foreground text-xs tabular-nums'>
          {t('userPortal.catalog.priceUpdated', { date: price.updatedAt })}
        </p>
      )}
    </div>
  )
}

export function ModelConnection({ model }: { model: PersonalModel }) {
  const { t } = useTranslation()
  const locationSearch = useLocation({ select: (loc) => loc.searchStr })
  const navigate = useNavigate()
  const requestedProtocol = readModelContext(locationSearch).protocol
  const protocol = model.protocols.includes(requestedProtocol as never)
    ? requestedProtocol
    : model.protocols[0]
  const baseURL = window.location.origin + '/v1'
  function selectProtocol(next: string) {
    if (!model.protocols.includes(next as never)) return
    void navigate({
      href: modelContextHref(locationSearch, {
        model: model.id,
        protocol: next as PersonalProtocol,
      }),
    })
  }
  return (
    <div className='space-y-4'>
      <ReferencePrice model={model} />
      <div className='bg-muted/40 space-y-3 rounded-lg border p-3'>
        <div className='space-y-1'>
          <h3 className='text-sm font-medium'>
            {t('userPortal.catalog.firstCall')}
          </h3>
          <p className='text-muted-foreground text-xs leading-relaxed'>
            {t('userPortal.catalog.exampleHint')}
          </p>
        </div>
        <div className='space-y-2'>
          <Label htmlFor='model-base-url' className='text-xs'>
            {t('userPortal.endpoint')}
          </Label>
          <Input
            id='model-base-url'
            value={baseURL}
            readOnly
            className='bg-background font-mono text-xs'
          />
          <div className='flex flex-wrap gap-2'>
            <CopyValue value={baseURL} label={t('userPortal.copyEndpoint')} />
            <CopyValue value={model.id} label={t('userPortal.copyModel')} />
          </div>
        </div>
      </div>
      {protocol ? (
        <div className='min-w-0 space-y-3'>
          <Tabs value={protocol} onValueChange={selectProtocol}>
            <TabsList
              className='grid w-full grid-cols-2 gap-1 group-data-horizontal/tabs:h-auto sm:inline-flex sm:w-fit'
              aria-label={t('userPortal.catalog.protocol')}
            >
              {model.protocols.map((p) => (
                <TabsTrigger key={p} value={p} className='h-auto min-h-8'>
                  {p}
                </TabsTrigger>
              ))}
            </TabsList>
            <TabsContent value={protocol} className='space-y-3'>
              <p className='text-muted-foreground text-xs leading-relaxed'>
                {t('userPortal.catalog.environmentKey')}
              </p>
              <pre
                className='bg-muted max-h-72 overflow-auto rounded-lg border p-3 font-mono text-xs'
                tabIndex={0}
                aria-label={t('userPortal.catalog.example')}
              >
                <code>
                  {connectionExample(
                    window.location.origin,
                    model.id,
                    protocol
                  )}
                </code>
              </pre>
              <CopyValue
                value={connectionExample(
                  window.location.origin,
                  model.id,
                  protocol
                )}
                label={t('userPortal.catalog.copyExample')}
              />
            </TabsContent>
          </Tabs>
        </div>
      ) : (
        <Notice tone='info' size='compact'>
          {t('userPortal.catalog.noExample')}
        </Notice>
      )}
      <Notice tone='info' size='compact'>
        {t('userPortal.catalog.verifyHint')}
      </Notice>
      <div className='flex flex-wrap gap-2'>
        <Button
          render={
            <Link
              to='/user'
              search={{
                view: 'keys',
                page: 1,
                model: model.id,
                protocol: protocol as PersonalProtocol | undefined,
              }}
            />
          }
        >
          {t('userPortal.createKey')}
          <ArrowRight aria-hidden='true' />
        </Button>
        <Button
          variant='outline'
          render={<Link to='/user' search={{ view: 'requests', page: 1 }} />}
        >
          {t('userPortal.nav.requests')}
        </Button>
      </div>
    </div>
  )
}
