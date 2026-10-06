import { FileJson, FileUp, ChevronDown, Code2, ShieldCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'

type Props = {
  raw: string
  fileName: string
  originKey: string
  external: boolean
  disabled: boolean
  onFile: (file: File) => void
  onText: (raw: string) => void
  onOrigin: (origin: string) => void
}

export function BackupImportSource(props: Props) {
  const { t } = useTranslation()
  return (
    <div className='space-y-4'>
      <label
        htmlFor='backup-import-file'
        className='group bg-primary/5 hover:bg-primary/10 border-primary/25 focus-within:ring-ring flex cursor-pointer flex-col items-center gap-3 rounded-xl border border-dashed px-5 py-7 text-center transition-colors focus-within:ring-2'
      >
        <span className='bg-background text-primary ring-border grid size-12 place-items-center rounded-xl shadow-sm ring-1'>
          {props.fileName ? (
            <FileJson className='size-6' aria-hidden='true' />
          ) : (
            <FileUp className='size-6' aria-hidden='true' />
          )}
        </span>
        <span className='max-w-full min-w-0 space-y-1'>
          <span className='block truncate text-sm font-semibold'>
            {props.fileName ||
              t('settings.content.importExport.design.uploadTitle')}
          </span>
          <span className='text-muted-foreground block text-sm'>
            {props.fileName
              ? t('settings.content.importExport.design.replaceFile')
              : t('settings.content.importExport.design.uploadHint')}
          </span>
        </span>
        <span className='flex flex-wrap justify-center gap-2'>
          <Badge variant='outline'>Metapi</Badge>
          <Badge variant='outline'>Octopus v5</Badge>
          <Badge variant='secondary'>JSON · 20 MB</Badge>
        </span>
        <Input
          id='backup-import-file'
          type='file'
          className='sr-only'
          accept='.json,application/json'
          aria-label={t('settings.content.importExport.selectBackupFile')}
          disabled={props.disabled}
          onChange={(event) => {
            const file = event.target.files?.[0]
            if (file) props.onFile(file)
            event.target.value = ''
          }}
        />
      </label>
      <p className='text-muted-foreground flex items-start gap-2 text-xs leading-relaxed'>
        <ShieldCheck className='mt-0.5 size-3.5 shrink-0' aria-hidden='true' />
        {t('settings.content.importExport.design.previewSafety')}
      </p>
      <Collapsible className='rounded-lg border'>
        <CollapsibleTrigger
          render={<Button variant='ghost' />}
          className='group h-auto w-full justify-start gap-2 px-3 py-3 text-sm'
        >
          <Code2 className='text-muted-foreground size-4' aria-hidden='true' />
          {t('settings.content.importExport.design.pasteJson')}
          <ChevronDown
            className='text-muted-foreground ml-auto size-4 transition-transform group-data-panel-open:rotate-180'
            aria-hidden='true'
          />
        </CollapsibleTrigger>
        <CollapsibleContent className='px-3 pb-3'>
          <Textarea
            value={props.raw}
            onChange={(event) => props.onText(event.target.value)}
            disabled={props.disabled}
            rows={6}
            aria-label={t('settings.content.importExport.design.pasteJson')}
            placeholder='{ "version": "..." }'
            className='max-h-64 font-mono text-xs leading-relaxed'
          />
        </CollapsibleContent>
      </Collapsible>
      {props.external ? (
        <div className='bg-muted/40 space-y-2 rounded-lg border p-4'>
          <label className='text-sm font-medium' htmlFor='external-origin-key'>
            {t('settings.content.importExport.externalOriginKey')}
          </label>
          <Input
            id='external-origin-key'
            value={props.originKey}
            onChange={(event) => props.onOrigin(event.target.value)}
            disabled={props.disabled}
            placeholder={t(
              'settings.content.importExport.externalOriginKeyPlaceholder'
            )}
          />
          <p className='text-muted-foreground text-xs leading-relaxed'>
            {t('settings.content.importExport.externalOriginKeyHint')}
          </p>
        </div>
      ) : null}
    </div>
  )
}
