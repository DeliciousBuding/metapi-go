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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'

import type { TesterTemplate } from '../lib/templates'
import type { TesterFormValues } from '../lib/tester-schema'

export function TestTemplateFields({
  templates,
  applyTemplate,
  isRunning,
}: {
  templates: TesterTemplate[]
  applyTemplate: (id: string | null) => void
  isRunning: boolean
}) {
  const { t } = useTranslation()
  const form = useFormContext<TesterFormValues>()
  return (
    <>
      <FormItem>
        <FormLabel>{t('modelTester.template.label')}</FormLabel>
        <Select value='' onValueChange={applyTemplate} disabled={isRunning}>
          <FormControl>
            <SelectTrigger className='w-full'>
              <SelectValue
                placeholder={t('modelTester.template.placeholder')}
              />
            </SelectTrigger>
          </FormControl>
          <SelectContent>
            {templates.map((template) => (
              <SelectItem key={template.id} value={template.id}>
                {t(template.labelKey)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <FormDescription>{t('modelTester.template.hint')}</FormDescription>
      </FormItem>

      <FormField
        control={form.control}
        name='systemPrompt'
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t('modelTester.form.systemPrompt')}</FormLabel>
            <FormControl>
              <Textarea
                placeholder={t('modelTester.form.systemPromptPlaceholder')}
                className='min-h-20 resize-y'
                disabled={isRunning}
                {...field}
              />
            </FormControl>
            <FormMessage />
          </FormItem>
        )}
      />
    </>
  )
}
