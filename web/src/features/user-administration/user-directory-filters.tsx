import { useNavigate, useSearch } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '@/components/ui/select'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

export function UserDirectoryFilters() {
  const { t } = useTranslation()
  const search = useSearch({ from: '/_authenticated/users' })
  const navigate = useNavigate({ from: '/users' })
  const view = search.view ?? 'users'
  const updateSearch = (next: Partial<typeof search>, replace = false) => {
    void navigate({
      search: (previous) => ({ ...previous, ...next }),
      replace,
    })
  }
  return (
    <>
      <Tabs
        value={view}
        onValueChange={(value) =>
          updateSearch({
            view: value as 'users' | 'groups',
            q: undefined,
            status: undefined,
          })
        }
      >
        <TabsList aria-label={t('userAdmin.views')}>
          <TabsTrigger value='users'>{t('userAdmin.users')}</TabsTrigger>
          <TabsTrigger value='groups'>{t('userAdmin.groups')}</TabsTrigger>
        </TabsList>
      </Tabs>
      <div className='flex flex-wrap items-center gap-2'>
        <Input
          className='h-8 w-full sm:w-72'
          aria-label={t('userAdmin.search')}
          placeholder={t('userAdmin.search')}
          value={search.q ?? ''}
          onChange={(event) =>
            updateSearch({ q: event.target.value || undefined }, true)
          }
        />
        <Select
          value={search.status ?? 'all'}
          onValueChange={(value) =>
            updateSearch({
              status:
                value === 'all' ? undefined : (value as 'enabled' | 'disabled'),
            })
          }
        >
          <SelectTrigger size='sm' aria-label={t('userAdmin.status')}>
            <SelectValue>
              {t(
                search.status
                  ? `userAdmin.${search.status}`
                  : 'userAdmin.allStatuses'
              )}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>{t('userAdmin.allStatuses')}</SelectItem>
            <SelectItem value='enabled'>{t('userAdmin.enabled')}</SelectItem>
            <SelectItem value='disabled'>{t('userAdmin.disabled')}</SelectItem>
          </SelectContent>
        </Select>
        {(search.q || search.status) && (
          <Button
            size='sm'
            variant='ghost'
            onClick={() => updateSearch({ q: undefined, status: undefined })}
          >
            {t('userAdmin.reset')}
          </Button>
        )}
      </div>
    </>
  )
}
