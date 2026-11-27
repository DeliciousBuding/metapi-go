import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { api, type CatalogSource } from '@/lib/api'
import { toast } from '@/lib/toast'

export type SourceDialogState =
  | { mode: 'new' }
  | { mode: 'edit'; source: CatalogSource }

export function SourceDialog({
  state,
  onOpenChange,
  onSaved,
}: {
  state: SourceDialogState
  onOpenChange: (open: boolean) => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const isEdit = state.mode === 'edit'
  const [name, setName] = useState(isEdit ? state.source.name : '')
  const [url, setUrl] = useState(isEdit ? state.source.url : '')
  const [type, setType] = useState<'official' | 'custom'>(
    isEdit ? state.source.type : 'custom'
  )
  const [enabled, setEnabled] = useState(isEdit ? state.source.enabled : true)

  const saveMutation = useMutation({
    mutationFn: async () => {
      if (isEdit) {
        return api.updateCatalogSource(state.source.id, {
          name,
          url,
          type,
          enabled,
        })
      }
      return api.createCatalogSource({ name, url, type, enabled })
    },
    onSuccess: () => {
      toast.success(t('settings.proxyModels.catalogSources.toast.updated'))
      onSaved()
    },
    onError: (error: Error) =>
      toast.error(
        t('settings.proxyModels.catalogSources.toast.saveFailed', {
          message: error.message,
        })
      ),
  })

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle>
            {isEdit
              ? t('settings.proxyModels.catalogSources.editTitle')
              : t('settings.proxyModels.catalogSources.addTitle')}
          </DialogTitle>
          <DialogDescription>
            {t('settings.proxyModels.catalogSources.description')}
          </DialogDescription>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <div className='flex flex-col gap-1.5'>
            <Label htmlFor='catalog-source-name'>
              {t('settings.proxyModels.catalogSources.nameLabel')}
            </Label>
            <Input
              id='catalog-source-name'
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder={t(
                'settings.proxyModels.catalogSources.namePlaceholder'
              )}
            />
          </div>
          <div className='flex flex-col gap-1.5'>
            <Label htmlFor='catalog-source-url'>
              {t('settings.proxyModels.catalogSources.urlLabel')}
            </Label>
            <Input
              id='catalog-source-url'
              value={url}
              onChange={(event) => setUrl(event.target.value)}
              placeholder={t(
                'settings.proxyModels.catalogSources.urlPlaceholder'
              )}
            />
            <p className='text-muted-foreground text-xs'>
              {t('settings.proxyModels.catalogSources.urlHint')}
            </p>
          </div>
          <div className='flex flex-col gap-1.5'>
            <Label>{t('settings.proxyModels.catalogSources.typeLabel')}</Label>
            <Select
              value={type}
              onValueChange={(value) => setType(value as 'official' | 'custom')}
            >
              <SelectTrigger className='w-full'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='custom'>
                  {t('settings.proxyModels.catalogSources.typeCustom')}
                </SelectItem>
                <SelectItem value='official'>
                  {t('settings.proxyModels.catalogSources.typeOfficial')}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className='flex items-center justify-between gap-2'>
            <Label htmlFor='catalog-source-enabled'>
              {t('settings.proxyModels.catalogSources.enabledLabel')}
            </Label>
            <Switch
              id='catalog-source-enabled'
              checked={enabled}
              onCheckedChange={setEnabled}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={() => onOpenChange(false)}>
            {t('settings.proxyModels.catalogSources.cancel')}
          </Button>
          <Button
            onClick={() => saveMutation.mutate()}
            disabled={saveMutation.isPending || !name.trim() || !url.trim()}
          >
            {t('settings.proxyModels.catalogSources.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
