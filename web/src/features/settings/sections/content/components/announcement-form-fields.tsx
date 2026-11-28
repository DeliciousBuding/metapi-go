import type { Control } from 'react-hook-form'
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
import { Textarea } from '@/components/ui/textarea'

import type { AnnouncementFormValues } from './announcement-form'

export function AnnouncementFormFields({
  control,
}: {
  control: Control<AnnouncementFormValues>
}) {
  const { t } = useTranslation()
  return (
    <>
      <FormField
        control={control}
        name='title'
        render={({ field }) => (
          <FormItem>
            <FormLabel>
              {t('settings.content.announcements.fields.title')}
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
        name='message'
        render={({ field }) => (
          <FormItem>
            <FormLabel>
              {t('settings.content.announcements.fields.message')}
            </FormLabel>
            <FormControl>
              <Textarea {...field} value={field.value ?? ''} rows={4} />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={control}
        name='severity'
        render={({ field }) => (
          <FormItem>
            <FormLabel>
              {t('settings.content.announcements.fields.severity')}
            </FormLabel>
            <Select value={field.value} onValueChange={field.onChange}>
              <FormControl>
                <SelectTrigger>
                  <SelectValue>
                    {(selected) =>
                      selected
                        ? t(
                            `settings.content.announcements.severity.${String(selected)}`
                          )
                        : ''
                    }
                  </SelectValue>
                </SelectTrigger>
              </FormControl>
              <SelectContent>
                <SelectItem value='info'>
                  {t('settings.content.announcements.severity.info')}
                </SelectItem>
                <SelectItem value='warning'>
                  {t('settings.content.announcements.severity.warning')}
                </SelectItem>
                <SelectItem value='critical'>
                  {t('settings.content.announcements.severity.critical')}
                </SelectItem>
              </SelectContent>
            </Select>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={control}
        name='link'
        render={({ field }) => (
          <FormItem>
            <FormLabel>
              {t('settings.content.announcements.fields.link')}
            </FormLabel>
            <FormControl>
              <Input
                {...field}
                value={field.value ?? ''}
                placeholder='https://…'
              />
            </FormControl>
            <FormDescription>
              {t('settings.content.announcements.fields.linkHint')}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
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
              {t('settings.content.announcements.fields.enabled')}
            </FormLabel>
          </FormItem>
        )}
      />
    </>
  )
}
