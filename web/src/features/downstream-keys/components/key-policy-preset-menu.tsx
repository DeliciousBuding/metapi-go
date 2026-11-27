import { Sparkles } from 'lucide-react'
import { useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { toast } from '@/lib/toast'

import {
  KEY_POLICY_PRESETS,
  type KeyPolicyPreset,
} from '../lib/key-policy-presets'
import type { CreateKeyFormValues } from './key-form-shared'

export function KeyPolicyPresetMenu() {
  const { t } = useTranslation()
  const form = useFormContext<CreateKeyFormValues>()
  function applyPreset(preset: KeyPolicyPreset) {
    form.setValue('supportedModels', [...preset.supportedModels], {
      shouldDirty: true,
      shouldValidate: true,
    })
    if (preset.maxRequests !== undefined) {
      form.setValue('maxRequests', preset.maxRequests, {
        shouldDirty: true,
        shouldValidate: true,
      })
    }
    if (preset.maxCost !== undefined) {
      form.setValue('maxCost', preset.maxCost, {
        shouldDirty: true,
        shouldValidate: true,
      })
    }
    toast.success(
      t('settings.downstream.keys.presets.applied', {
        name: t(preset.labelKey),
      })
    )
  }

  return (
    <div className='bg-muted/20 flex flex-wrap items-center justify-between gap-2 rounded-lg border px-3 py-2'>
      <p className='text-muted-foreground text-xs'>
        {t('settings.downstream.keys.presets.hint')}
      </p>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button type='button' variant='outline' size='xs'>
              <Sparkles className='size-3.5' aria-hidden='true' />
              {t('settings.downstream.keys.presets.label')}
            </Button>
          }
        />
        <DropdownMenuContent align='end'>
          {KEY_POLICY_PRESETS.map((preset) => (
            <DropdownMenuItem
              key={preset.id}
              onClick={() => applyPreset(preset)}
            >
              {t(preset.labelKey)}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
