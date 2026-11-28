import { useFormContext } from 'react-hook-form'
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

import { ScheduleEditor } from '../../../components/schedule-editor'
import type { WebdavFormValues } from './use-webdav-config'

export function WebdavConfigFields() {
  const { t } = useTranslation()
  const { control } = useFormContext<WebdavFormValues>()
  return (
    <>
      <FormField
        control={control}
        name='enabled'
        render={({ field }) => (
          <FormItem className='flex flex-row items-center gap-3'>
            <FormControl>
              <Switch
                checked={Boolean(field.value)}
                onCheckedChange={field.onChange}
              />
            </FormControl>
            <FormLabel className='cursor-pointer'>
              {t('settings.content.importExport.fields.webdavEnabled')}
            </FormLabel>
          </FormItem>
        )}
      />
      <FormField
        control={control}
        name='fileUrl'
        render={({ field }) => (
          <FormItem>
            <FormLabel>
              {t('settings.content.importExport.fields.webdavFileUrl')}
            </FormLabel>
            <FormControl>
              <Input
                {...field}
                value={field.value ?? ''}
                placeholder='https://dav.example.com/backups/metapi.json'
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
        <FormField
          control={control}
          name='username'
          render={({ field }) => (
            <FormItem>
              <FormLabel>
                {t('settings.content.importExport.fields.webdavUsername')}
              </FormLabel>
              <FormControl>
                <Input {...field} value={field.value ?? ''} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={control}
          name='password'
          render={({ field }) => (
            <FormItem>
              <FormLabel>
                {t('settings.content.importExport.fields.webdavPassword')}
              </FormLabel>
              <FormControl>
                <Input
                  {...field}
                  value={field.value ?? ''}
                  type='password'
                  placeholder={t(
                    'settings.content.importExport.fields.webdavPasswordHint'
                  )}
                />
              </FormControl>
              <FormDescription>
                {t(
                  'settings.content.importExport.fields.webdavPasswordDescription'
                )}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>
      <FormField
        control={control}
        name='exportType'
        render={({ field }) => (
          <FormItem>
            <FormLabel>
              {t('settings.content.importExport.fields.webdavExportType')}
            </FormLabel>
            <Select value={field.value ?? 'all'} onValueChange={field.onChange}>
              <FormControl>
                <SelectTrigger>
                  <SelectValue>
                    {(selected) => {
                      const labels: Record<string, string> = {
                        all: t('settings.content.importExport.exportAll'),
                        accounts: t(
                          'settings.content.importExport.exportAccounts'
                        ),
                        preferences: t(
                          'settings.content.importExport.exportPreferences'
                        ),
                      }
                      return selected
                        ? (labels[String(selected)] ?? String(selected))
                        : ''
                    }}
                  </SelectValue>
                </SelectTrigger>
              </FormControl>
              <SelectContent>
                <SelectItem value='all'>
                  {t('settings.content.importExport.exportAll')}
                </SelectItem>
                <SelectItem value='accounts'>
                  {t('settings.content.importExport.exportAccounts')}
                </SelectItem>
                <SelectItem value='preferences'>
                  {t('settings.content.importExport.exportPreferences')}
                </SelectItem>
              </SelectContent>
            </Select>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={control}
        name='autoSyncEnabled'
        render={({ field }) => (
          <FormItem className='flex flex-row items-center gap-3'>
            <FormControl>
              <Switch
                checked={Boolean(field.value)}
                onCheckedChange={field.onChange}
              />
            </FormControl>
            <FormLabel className='cursor-pointer'>
              {t('settings.content.importExport.fields.webdavAutoSyncEnabled')}
            </FormLabel>
          </FormItem>
        )}
      />
      <FormField
        control={control}
        name='autoSyncSchedule'
        render={({ field }) => (
          <FormItem>
            <FormLabel>
              {t('settings.content.importExport.fields.webdavAutoSyncCron')}
            </FormLabel>
            <FormControl>
              <ScheduleEditor value={field.value} onChange={field.onChange} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  )
}
