import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Form,
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

import type { MigrationFormValues } from './use-database-migration-form'

export function DatabaseMigrationForm({
  form,
  busy,
  testing,
  migrating,
  onTestConnection,
  onStartMigration,
}: {
  form: UseFormReturn<MigrationFormValues>
  busy: boolean
  testing: boolean
  migrating: boolean
  onTestConnection: () => void
  onStartMigration: () => void
}) {
  const { t } = useTranslation()
  const dialect = form.watch('dialect')
  return (
    <Form {...form}>
      <form className='space-y-4'>
        <FormField
          control={form.control}
          name='dialect'
          render={({ field }) => (
            <FormItem>
              <FormLabel>
                {t('settings.operations.database.migration.fields.dialect')}
              </FormLabel>
              <Select
                value={field.value}
                disabled={busy}
                onValueChange={(value) => {
                  field.onChange(value)
                  if (value === 'sqlite') {
                    form.setValue('ssl', false, { shouldDirty: true })
                  }
                }}
              >
                <FormControl>
                  <SelectTrigger>
                    <SelectValue>
                      {(selected) => {
                        if (selected === 'sqlite') return 'SQLite'
                        if (selected === 'postgres') return 'PostgreSQL'
                        return ''
                      }}
                    </SelectValue>
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  <SelectItem value='sqlite'>SQLite</SelectItem>
                  <SelectItem value='postgres'>PostgreSQL</SelectItem>
                </SelectContent>
              </Select>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='connectionString'
          render={({ field }) => (
            <FormItem>
              <FormLabel>
                {t(
                  'settings.operations.database.migration.fields.connectionString'
                )}
              </FormLabel>
              <FormControl>
                <Input
                  {...field}
                  value={field.value ?? ''}
                  className='font-mono'
                  // Infrastructure DSN, not a user credential — WCAG 1.3.5
                  // input purposes do not apply, and 'off' keeps password
                  // managers from capturing or autofilling connection
                  // strings (#1029 batch A evaluation).
                  autoComplete='off'
                  spellCheck={false}
                  disabled={busy}
                  placeholder={
                    dialect === 'postgres'
                      ? 'postgres://user@host:5432/db'
                      : './data/target.db'
                  }
                />
              </FormControl>
              <FormDescription>
                {t(
                  'settings.operations.database.migration.fields.connectionStringHint'
                )}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        {dialect === 'postgres' ? (
          <FormField
            control={form.control}
            name='ssl'
            render={({ field }) => (
              <FormItem className='flex flex-row items-start gap-3 rounded-lg border p-3'>
                <FormControl>
                  <Checkbox
                    checked={field.value}
                    disabled={busy}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
                <div className='space-y-1'>
                  <FormLabel className='cursor-pointer'>
                    {t('settings.operations.database.migration.fields.ssl')}
                  </FormLabel>
                  <FormDescription>
                    {t('settings.operations.database.migration.fields.sslHint')}
                  </FormDescription>
                </div>
              </FormItem>
            )}
          />
        ) : null}
        <FormField
          control={form.control}
          name='overwrite'
          render={({ field }) => (
            <FormItem className='flex flex-row items-start gap-3 rounded-lg border p-3'>
              <FormControl>
                <Checkbox
                  checked={field.value}
                  disabled={busy}
                  onCheckedChange={field.onChange}
                />
              </FormControl>
              <div className='space-y-1'>
                <FormLabel className='cursor-pointer'>
                  {t('settings.operations.database.migration.fields.overwrite')}
                </FormLabel>
                <FormDescription>
                  {t(
                    'settings.operations.database.migration.fields.overwriteHint'
                  )}
                </FormDescription>
              </div>
            </FormItem>
          )}
        />

        <div className='flex flex-col gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between'>
          <div className='flex flex-wrap gap-2'>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={busy}
              onClick={onTestConnection}
            >
              {testing
                ? t('settings.common.testing')
                : t('settings.operations.database.migration.testConnection')}
            </Button>
            <Button
              type='button'
              variant='destructive'
              size='sm'
              disabled={busy}
              onClick={onStartMigration}
            >
              {migrating
                ? t('settings.operations.database.migration.migrating')
                : t('settings.operations.database.migration.startMigration')}
            </Button>
          </div>
        </div>
      </form>
    </Form>
  )
}
