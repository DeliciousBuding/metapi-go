import { CalendarRange } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { DateRangePresets } from '@/components/common/date-range-presets'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { Account } from '@/features/accounts'

import type { useCheckinUrlState } from './use-checkin-url-state'

const ACCOUNT_FILTER_ALL = 'all'

export function CheckinFilterBar({
  accountOptions,
  accountId,
  from,
  to,
  updateUrlState,
  hasActiveFilters,
  handleResetFilters,
}: {
  accountOptions: Account[]
  accountId?: number
  from: string
  to: string
  updateUrlState: ReturnType<typeof useCheckinUrlState>['updateUrlState']
  hasActiveFilters: boolean
  handleResetFilters: () => void
}) {
  const { t } = useTranslation()
  return (
    <div className='flex flex-wrap items-center gap-2'>
      {/* Native datetime controls render in the BROWSER's locale (not
              the page's): zh-CN browsers get 年月日/24h, en-US browsers get
              mm/dd/yyyy. What we own is the layout: on mobile the pair stacks
              full-width so neither clips, and the 至 separator hides instead
              of orphaning at a wrapped line end. */}
      <CalendarRange
        aria-hidden='true'
        className='text-muted-foreground size-4 max-sm:hidden'
      />
      <div className='flex items-center gap-2 max-sm:w-full max-sm:flex-col max-sm:items-stretch'>
        <Input
          type='datetime-local'
          value={from}
          onChange={(event) => {
            updateUrlState({
              filters: { from: event.target.value },
              pageIndex: 0,
            })
          }}
          className='w-50 max-sm:w-full'
          aria-label={t('checkin.page.startTime')}
        />
        <span className='text-muted-foreground text-sm max-sm:hidden'>
          {t('checkin.page.to')}
        </span>
        <Input
          type='datetime-local'
          value={to}
          onChange={(event) => {
            updateUrlState({
              filters: { to: event.target.value },
              pageIndex: 0,
            })
          }}
          className='w-50 max-sm:w-full'
          aria-label={t('checkin.page.endTime')}
        />
      </div>
      <DateRangePresets
        onApply={(fromValue, toValue) => {
          updateUrlState({
            filters: { from: fromValue, to: toValue },
            pageIndex: 0,
          })
        }}
      />
      <Select
        value={accountId ? String(accountId) : ACCOUNT_FILTER_ALL}
        onValueChange={(value) => {
          updateUrlState({
            filters: {
              accountId: !value || value === ACCOUNT_FILTER_ALL ? '' : value,
            },
            pageIndex: 0,
          })
        }}
      >
        <SelectTrigger
          aria-label={t('checkin.page.filterAccountTitle')}
          className='w-50 max-sm:w-full'
        >
          <SelectValue>
            {(selected) => {
              if (!selected || selected === ACCOUNT_FILTER_ALL) {
                return t('checkin.page.allAccounts')
              }
              const account = accountOptions.find(
                (item) => String(item.id) === selected
              )
              return account
                ? account.username || `#${account.id}`
                : String(selected)
            }}
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ACCOUNT_FILTER_ALL}>
            {t('checkin.page.allAccounts')}
          </SelectItem>
          {accountOptions.map((account) => (
            <SelectItem key={account.id} value={String(account.id)}>
              {account.username || `#${account.id}`}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {hasActiveFilters && (
        <Button variant='ghost' size='sm' onClick={handleResetFilters}>
          {t('checkin.page.resetFilters')}
        </Button>
      )}
    </div>
  )
}
