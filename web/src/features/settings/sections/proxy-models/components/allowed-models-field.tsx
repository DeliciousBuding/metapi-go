import { useTranslation } from 'react-i18next'

import { ModelPill } from '@/components/common/model-pill'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import type { useModelAllowlist } from './use-model-allowlist'

export function AllowedModelsField({
  control,
  models,
}: {
  control: ReturnType<typeof useModelAllowlist>['form']['control']
  models: ReturnType<typeof useModelAllowlist>['models']
}) {
  const { t } = useTranslation()
  const {
    pendingModel,
    setPendingModel,
    allowedModels,
    candidateModels,
    addAllowedModel,
    removeAllowedModel,
  } = models
  return (
    <FormField
      control={control}
      name='globalAllowedModels'
      render={({ field }) => (
        <FormItem>
          <FormLabel>
            {t('settings.proxyModels.allowlist.fields.globalAllowedModels')}
          </FormLabel>
          <FormDescription>
            {t('settings.proxyModels.allowlist.fields.globalAllowedModelsHint')}
          </FormDescription>
          <div className='flex flex-wrap gap-2'>
            {allowedModels.map((model) => (
              <Badge
                key={model}
                variant='default'
                render={<button type='button' />}
                className='h-auto min-h-6 cursor-pointer'
                aria-label={t('settings.downstream.keys.access.removeModel', {
                  model,
                })}
                onClick={() => removeAllowedModel(model)}
              >
                <ModelPill model={model} variant='inline' /> ×
              </Badge>
            ))}
          </div>
          <div className='flex gap-2'>
            <FormControl>
              <Input
                value={pendingModel}
                onChange={(event) => setPendingModel(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    event.preventDefault()
                    if (
                      event.nativeEvent.isComposing ||
                      event.nativeEvent.keyCode === 229
                    )
                      return
                    addAllowedModel(pendingModel)
                  }
                }}
                placeholder={t('settings.proxyModels.allowlist.addPlaceholder')}
              />
            </FormControl>
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => addAllowedModel(pendingModel)}
            >
              {t('settings.common.add')}
            </Button>
          </div>
          {candidateModels.length > 0 ? (
            <div className='flex flex-wrap gap-1'>
              {candidateModels.slice(0, 20).map((model) => (
                <button
                  type='button'
                  key={model}
                  className='cursor-pointer'
                  onClick={() => addAllowedModel(model)}
                >
                  <Badge variant='secondary'>
                    + <ModelPill model={model} variant='inline' />
                  </Badge>
                </button>
              ))}
            </div>
          ) : null}
          <input type='hidden' {...field} />
          <FormMessage />
        </FormItem>
      )}
    />
  )
}
