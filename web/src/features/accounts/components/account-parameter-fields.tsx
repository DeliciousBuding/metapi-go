import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import type { AccountFormValues } from '../lib/accounts-schema'

export function AccountParameterFields({
  form,
}: {
  form: UseFormReturn<AccountFormValues>
}) {
  const { t } = useTranslation()
  return (
    <>
      {/* Status */}
      <FormField
        control={form.control}
        name='status'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.form.status')}</FormLabel>
            <Select value={field.value} onValueChange={field.onChange}>
              <FormControl>
                <SelectTrigger>
                  <SelectValue>
                    {(selected) => {
                      const labels: Record<string, string> = {
                        active: t('accounts.form.statusActive'),
                        disabled: t('accounts.form.statusDisabled'),
                        expired: t('accounts.form.statusExpired'),
                      }
                      return selected
                        ? (labels[String(selected)] ?? String(selected))
                        : ''
                    }}
                  </SelectValue>
                </SelectTrigger>
              </FormControl>
              <SelectContent>
                <SelectItem value='active'>
                  {t('accounts.form.statusActive')}
                </SelectItem>
                <SelectItem value='disabled'>
                  {t('accounts.form.statusDisabled')}
                </SelectItem>
                <SelectItem value='expired'>
                  {t('accounts.form.statusExpired')}
                </SelectItem>
              </SelectContent>
            </Select>
            <FormMessage />
          </FormItem>
        )}
      />

      {/* Checkin toggle */}
      <FormField
        control={form.control}
        name='checkinEnabled'
        render={({ field }) => (
          <FormItem className='flex flex-row items-center justify-between rounded-lg border p-3'>
            <div className='space-y-0.5'>
              <FormLabel>{t('accounts.form.checkinEnabled')}</FormLabel>
              <FormDescription>
                {t('accounts.form.checkinEnabledHint')}
              </FormDescription>
            </div>
            <FormControl>
              <Switch checked={field.value} onCheckedChange={field.onChange} />
            </FormControl>
          </FormItem>
        )}
      />

      {/* Unit cost */}
      <FormField
        control={form.control}
        name='unitCost'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.form.unitCost')}</FormLabel>
            <FormControl>
              <Input
                type='number'
                step='0.01'
                placeholder='0.00'
                value={field.value ?? ''}
                onChange={(event) =>
                  field.onChange(
                    event.target.value === ''
                      ? undefined
                      : Number(event.target.value)
                  )
                }
                onBlur={field.onBlur}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      {/* Proxy URL */}
      <FormField
        control={form.control}
        name='proxyUrl'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.form.proxyUrl')}</FormLabel>
            <FormControl>
              <Input
                placeholder='https://proxy.example.com or socks5://proxy.example.com:1080'
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />

      {/* Tags */}
      <FormField
        control={form.control}
        name='tags'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('accounts.form.tags')}</FormLabel>
            <FormControl>
              <Input
                placeholder='prod, priority'
                {...field}
                value={field.value ?? ''}
              />
            </FormControl>
            <FormDescription>{t('accounts.form.tagsHint')}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  )
}
