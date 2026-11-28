import { Search as SearchIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Notice } from '@/components/ui/notice'
import { Separator } from '@/components/ui/separator'
import { Spinner } from '@/components/ui/spinner'

import { analyzePrimarySiteUrl } from '../lib/site-primary-url'
import type { Site } from '../types'
import { EndpointsEditor } from './endpoints-editor'
import { SiteConnectionTemplates } from './site-connection-templates'
import { SiteFormPlatformField } from './site-form-platform-field'
import type { useSiteConnectionForm } from './use-site-connection-form'

export function SiteFormConnectionFields({
  connection,
  editingSite,
}: {
  connection: ReturnType<typeof useSiteConnectionForm>
  editingSite: Site | null
}) {
  const { t } = useTranslation()
  const {
    form,
    selectedTemplate,
    templateFeedback,
    watchedPlatform,
    watchedUrl,
    handlePreset,
    platformMode,
    setPlatformMode,
    handleDetect,
    detecting,
  } = connection
  const isEditing = editingSite !== null
  const urlAnalysis = analyzePrimarySiteUrl(watchedUrl)
  return (
    <>
      {!isEditing && (
        <>
          <SiteConnectionTemplates
            selected={selectedTemplate}
            feedback={templateFeedback}
            url={watchedUrl}
            platform={watchedPlatform}
            onSelect={handlePreset}
          />
          <Separator className='my-6' />
        </>
      )}
      <fieldset className='min-w-0 space-y-4'>
        <legend className='mb-4 text-sm font-semibold'>
          {t('sites.form.sections.connection')}
        </legend>
        <div className='grid items-start gap-x-5 gap-y-4 sm:grid-cols-2'>
          <FormField
            control={form.control}
            name='name'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('sites.form.name')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('sites.form.namePlaceholder')}
                    autoFocus={isEditing}
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <SiteFormPlatformField
            control={form.control}
            platformMode={platformMode}
            setPlatformMode={setPlatformMode}
          />
        </div>

        <FormField
          control={form.control}
          name='url'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('sites.form.url')}</FormLabel>
              <div className='flex gap-2'>
                <FormControl>
                  <Input
                    placeholder='https://example.com'
                    className='flex-1'
                    {...field}
                  />
                </FormControl>
                <Button
                  type='button'
                  variant='outline'
                  onClick={handleDetect}
                  disabled={detecting}
                >
                  {detecting ? (
                    <Spinner />
                  ) : (
                    <SearchIcon className='size-3.5' />
                  )}
                  {t('sites.form.detect')}
                </Button>
              </div>
              {selectedTemplate?.group !== 'api' &&
                urlAnalysis.action === 'auto_strip_known_api_suffix' &&
                urlAnalysis.persistedUrl && (
                  <Notice tone='info' size='compact'>
                    {t('sites.form.urlAutoStripInfo', {
                      url: urlAnalysis.persistedUrl,
                    })}
                  </Notice>
                )}
              {selectedTemplate?.group !== 'api' &&
                urlAnalysis.action === 'preserve_api_path' &&
                urlAnalysis.persistedUrl && (
                  <Notice tone='warning' size='compact'>
                    {t('sites.form.urlPreserveApiPath')}
                  </Notice>
                )}
              {selectedTemplate?.group !== 'api' &&
                urlAnalysis.action === 'preserve_unknown_path' &&
                urlAnalysis.persistedUrl && (
                  <Notice tone='warning' size='compact'>
                    {t('sites.form.urlPreserveUnknownPath')}
                  </Notice>
                )}
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name='apiEndpointsText'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('sites.form.apiEndpoints')}</FormLabel>
              <FormControl>
                <EndpointsEditor
                  value={field.value}
                  onChange={field.onChange}
                  liveEndpoints={editingSite?.apiEndpoints}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <FormField
          control={form.control}
          name='externalCheckinUrl'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('sites.form.externalCheckinUrl')}</FormLabel>
              <FormControl>
                <Input
                  placeholder={t('sites.form.optionalUrlPlaceholder')}
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </fieldset>
    </>
  )
}
