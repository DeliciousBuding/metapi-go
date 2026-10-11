import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

export function ScopePickerToolbar(props: {
  selectedCount: number
  search: string
  onSearchChange: (search: string) => void
  onlySelected: boolean
  onOnlySelectedChange: (selected: boolean) => void
  onLocateSelected: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className='space-y-2'>
      <div className='flex flex-wrap items-center gap-2'>
        <Badge variant={props.selectedCount > 0 ? 'secondary' : 'outline'}>
          {t('settings.downstream.keys.picker.selected', {
            count: props.selectedCount,
          })}
        </Badge>
        <Button
          type='button'
          variant='outline'
          size='xs'
          aria-pressed={props.onlySelected}
          onClick={() => props.onOnlySelectedChange(!props.onlySelected)}
        >
          {t('settings.downstream.keys.picker.onlySelected')}
        </Button>
        <Button
          type='button'
          variant='ghost'
          size='xs'
          disabled={props.selectedCount === 0}
          onClick={props.onLocateSelected}
        >
          {t('settings.downstream.keys.picker.locateSelected')}
        </Button>
      </div>
      <Input
        value={props.search}
        onChange={(event) => props.onSearchChange(event.target.value)}
        aria-label={t('settings.downstream.keys.picker.search')}
        placeholder={t('settings.downstream.keys.picker.search')}
      />
    </div>
  )
}
