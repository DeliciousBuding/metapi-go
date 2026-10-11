import { useState, type ComponentProps } from 'react'
import { useTranslation } from 'react-i18next'

import { formLabelIdFor } from '@/components/ui/form'

import { useScopeSelectionFocus } from '../hooks/use-scope-selection-focus'
import { ScopePickerToolbar } from './scope-picker-toolbar'

// Legacy site allow-lists: empty means no additional site restriction.
export function SiteScopePicker({
  value,
  onChange,
  sites,
  ...props
}: {
  value: number[]
  onChange: (siteIds: number[]) => void
  sites: Array<{ id: number; name: string }>
} & Omit<ComponentProps<'div'>, 'onChange'>) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const [onlySelected, setOnlySelected] = useState(false)
  const focus = useScopeSelectionFocus()
  const selected = new Set(value)
  const options = [
    ...sites,
    ...value
      .filter((id) => !sites.some((site) => site.id === id))
      .map((id) => ({
        id,
        name: t('settings.downstream.keys.scope.unknownId', { id }),
      })),
  ].filter(
    (site) =>
      (!onlySelected || selected.has(site.id)) &&
      `${site.name} ${site.id}`
        .toLocaleLowerCase()
        .includes(search.trim().toLocaleLowerCase())
  )
  return (
    <div
      className='space-y-2'
      data-testid='site-scope-picker'
      role='group'
      aria-labelledby={props.id ? formLabelIdFor(props.id) : undefined}
      {...props}
    >
      <ScopePickerToolbar
        selectedCount={selected.size}
        search={search}
        onSearchChange={setSearch}
        onlySelected={onlySelected}
        onOnlySelectedChange={setOnlySelected}
        onLocateSelected={() => {
          setSearch('')
          setOnlySelected(true)
          focus.focusSelected()
        }}
      />
      <div
        ref={focus.listRef}
        className='border-border max-h-40 space-y-1 overflow-y-auto rounded-md border p-2'
      >
        {options.map((site) => (
          <label
            key={site.id}
            className='flex cursor-pointer items-center gap-2 text-sm'
          >
            <input
              type='checkbox'
              checked={selected.has(site.id)}
              onChange={(event) => {
                const next = new Set(selected)
                if (event.target.checked) next.add(site.id)
                else next.delete(site.id)
                onChange([...next].sort((left, right) => left - right))
              }}
            />
            <span className='min-w-0 break-words'>{site.name}</span>
          </label>
        ))}
        {options.length === 0 && (
          <p className='text-muted-foreground text-xs'>
            {t('settings.downstream.keys.picker.noMatches')}
          </p>
        )}
      </div>
    </div>
  )
}
