import type { ComponentProps } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import type { ProviderSettingsProps } from './channel-provider-protocol-fields'

export function ProviderCatalogFields({
  value,
  manifest,
  blocked,
  update,
  id,
  newChannel,
  invalid,
  onRemove,
}: ProviderSettingsProps & {
  id: string
  newChannel?: boolean
  invalid?: ComponentProps<'input'>['aria-invalid']
} & { onRemove: () => void }) {
  const { t } = useTranslation()
  const catalog = value.catalog
  return (
    <>
      {catalog ? (
        <div className='space-y-2 border-t pt-3'>
          <label htmlFor={`${id}-catalog`} className='text-sm'>
            {t('channels.plugins.catalogUrl')}
          </label>
          <Input
            id={`${id}-catalog`}
            value={catalog.url}
            aria-invalid={invalid}
            disabled={blocked}
            onChange={(event) =>
              update({
                catalog: { ...catalog, url: event.target.value },
              })
            }
          />
          <label
            id={`${id}-auth-label`}
            htmlFor={`${id}-auth`}
            className='text-sm'
          >
            {t('channels.plugins.catalogAuth')}
          </label>
          <Select
            value={catalog.auth}
            disabled={blocked}
            onValueChange={(auth) => {
              if (
                auth === 'bearer' ||
                auth === 'x-api-key' ||
                auth === 'x-goog-api-key' ||
                auth === 'none'
              )
                update({ catalog: { ...catalog, auth } })
            }}
          >
            <SelectTrigger id={`${id}-auth`}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent aria-labelledby={`${id}-auth-label`}>
              {(['bearer', 'x-api-key', 'x-goog-api-key', 'none'] as const).map(
                (auth) => (
                  <SelectItem key={auth} value={auth}>
                    {auth}
                  </SelectItem>
                )
              )}
            </SelectContent>
          </Select>
          <label htmlFor={`${id}-cursor`} className='text-sm'>
            {t('channels.plugins.catalogCursor')}
          </label>
          <Input
            id={`${id}-cursor`}
            value={catalog.cursorParameter ?? ''}
            disabled={blocked}
            onChange={(event) =>
              update({
                catalog: {
                  ...catalog,
                  cursorParameter: event.target.value,
                },
              })
            }
          />
          <Button
            type='button'
            size='xs'
            variant='outline'
            disabled={blocked}
            onClick={onRemove}
          >
            {t(
              newChannel && manifest?.defaults?.catalog
                ? 'channels.plugins.catalogUseDefault'
                : 'channels.plugins.catalogRemove'
            )}
          </Button>
        </div>
      ) : (
        manifest?.hooks.includes('modelsRequest') && (
          <div className='space-y-2'>
            {newChannel && manifest.defaults?.catalog && (
              <p className='text-muted-foreground text-sm'>
                {t('channels.plugins.catalogDefault', {
                  path: manifest.defaults.catalog.path,
                })}
              </p>
            )}
            <Button
              type='button'
              size='xs'
              variant='outline'
              disabled={blocked}
              onClick={() => update({ catalog: { url: '', auth: 'bearer' } })}
            >
              {t('channels.plugins.catalogConfigure')}
            </Button>
          </div>
        )
      )}
      {(catalog || manifest?.hooks.includes('modelsRequest')) && (
        <p className='text-muted-foreground text-xs'>
          {t('channels.plugins.catalogHint')}
        </p>
      )}
    </>
  )
}
