import { Search } from 'lucide-react'
import { useId, useMemo, useState } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import {
  setChannelDraftSelection,
  type RouteFormValues,
} from '../lib/routes-schema'

export type RouteAccountOption = { id: number; label: string }

export function RouteChannelDraftField({
  form,
  accountOptions,
  isEdit,
  chainContext,
}: {
  form: UseFormReturn<RouteFormValues>
  accountOptions: RouteAccountOption[]
  isEdit: boolean
  chainContext?: { accountId?: number; tokenId?: number }
}) {
  const { t } = useTranslation()
  const bulkLabelId = useId()
  const [accountSearch, setAccountSearch] = useState('')
  const visibleAccounts = useMemo(() => {
    const query = accountSearch.trim().toLocaleLowerCase()
    return query
      ? accountOptions.filter((account) =>
          account.label.toLocaleLowerCase().includes(query)
        )
      : accountOptions
  }, [accountOptions, accountSearch])
  return (
    <FormField
      control={form.control}
      name='channelDrafts'
      render={({ field }) => {
        const selected = field.value ?? []
        const selectedIds = new Set(selected.map((draft) => draft.accountId))
        const accountIds = visibleAccounts.map((account) => account.id)
        const selectedCount = accountIds.filter((id) =>
          selectedIds.has(id)
        ).length
        const allSelected =
          accountIds.length > 0 && selectedCount === accountIds.length
        const someSelected = selectedCount > 0 && !allSelected
        const toggleAccounts = (ids: number[], checked: boolean) => {
          const next = setChannelDraftSelection(selected, ids, checked)
          // Re-selecting the originating account still means the chosen key,
          // not an implicit fallback to that account's default credential.
          field.onChange(
            next.map((draft) =>
              draft.accountId === chainContext?.accountId &&
              chainContext.tokenId
                ? { ...draft, tokenId: chainContext.tokenId }
                : draft
            )
          )
        }
        let selectKey = allSelected
          ? 'tokenRoutes.formPattern.channelsDeselectAll'
          : 'tokenRoutes.formPattern.channelsSelectAll'
        if (accountSearch.trim()) {
          selectKey = allSelected
            ? 'tokenRoutes.formPattern.channelsDeselectFiltered'
            : 'tokenRoutes.formPattern.channelsSelectFiltered'
        }
        const bulkLabel = t(selectKey)
        const totalSelected = accountOptions.filter((account) =>
          selectedIds.has(account.id)
        ).length
        return (
          <FormItem>
            <FormLabel>{t('tokenRoutes.formPattern.channels')}</FormLabel>
            <FormDescription>
              {t('tokenRoutes.formPattern.channelsHint')}
            </FormDescription>
            {accountOptions.length > 0 ? (
              <div className='overflow-hidden rounded-lg border'>
                <div className='relative border-b p-2'>
                  <Search
                    className='text-muted-foreground pointer-events-none absolute top-1/2 left-5 size-4 -translate-y-1/2'
                    aria-hidden='true'
                  />
                  <Input
                    type='search'
                    aria-label={t('tokenRoutes.formPattern.searchAccounts')}
                    placeholder={t('tokenRoutes.formPattern.searchAccounts')}
                    value={accountSearch}
                    onChange={(event) => setAccountSearch(event.target.value)}
                    className='border-0 bg-transparent pl-9 shadow-none'
                  />
                </div>
                <div className='bg-muted/40 flex flex-wrap items-center justify-between gap-2 border-b px-4 py-2'>
                  <label className='flex items-center gap-2 text-sm'>
                    <FormControl>
                      <Checkbox
                        ref={field.ref}
                        checked={allSelected}
                        disabled={accountIds.length === 0}
                        indeterminate={someSelected}
                        onCheckedChange={(checked) =>
                          toggleAccounts(accountIds, checked)
                        }
                        onBlur={field.onBlur}
                        aria-labelledby={bulkLabelId}
                      />
                    </FormControl>
                    <span id={bulkLabelId}>{bulkLabel}</span>
                  </label>
                  <span
                    role='status'
                    className='text-muted-foreground text-xs tabular-nums'
                  >
                    {t('tokenRoutes.formPattern.channelsSelectedCount', {
                      selected: totalSelected,
                      total: accountOptions.length,
                    })}
                  </span>
                </div>
                <div className='max-h-56 space-y-1 overflow-y-auto p-2'>
                  {visibleAccounts.length === 0 && (
                    <p
                      className='text-muted-foreground px-3 py-5 text-center text-sm'
                      role='status'
                    >
                      {t('tokenRoutes.formPattern.noMatchingAccounts')}
                    </p>
                  )}
                  {visibleAccounts.map((account) => (
                    <label
                      key={account.id}
                      className='hover:bg-muted flex min-h-9 items-center gap-3 rounded-md px-2.5 py-2'
                    >
                      <Checkbox
                        checked={selectedIds.has(account.id)}
                        onCheckedChange={(checked) =>
                          toggleAccounts([account.id], checked)
                        }
                      />
                      <span className='truncate text-sm'>{account.label}</span>
                      {selected
                        .filter(
                          (draft) =>
                            draft.accountId === account.id && draft.tokenId
                        )
                        .map((draft) => (
                          <span
                            key={draft.tokenId}
                            className='text-muted-foreground ml-auto shrink-0 text-xs'
                          >
                            {t('tokenRoutes.formPattern.selectedKey', {
                              id: draft.tokenId,
                            })}
                          </span>
                        ))}
                    </label>
                  ))}
                </div>
              </div>
            ) : (
              <div className='flex flex-col gap-2 rounded-lg border border-dashed p-3'>
                <p className='text-muted-foreground text-sm'>
                  {t(
                    isEdit
                      ? 'tokenRoutes.formPattern.channelsEmptyHintEdit'
                      : 'tokenRoutes.formPattern.channelsEmptyHint'
                  )}
                </p>
              </div>
            )}
            <FormMessage />
          </FormItem>
        )
      }}
    />
  )
}
