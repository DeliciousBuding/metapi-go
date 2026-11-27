import { KeyRound, Pencil, Trash2, Unplug } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toBcp47 } from '@/i18n/languages'
import type { ImportedCredential } from '@/lib/api/imported-upstreams'
import { formatDateTime } from '@/lib/format'

import { credentialKindLabel } from '../lib/upstream-config'
import { CredentialForm } from './upstream-credential-form'

export function CredentialRow({
  credential,
  expanded,
  hidden,
  deleting,
  onToggle,
  onDelete,
  onDirtyChange,
}: {
  credential: ImportedCredential
  expanded: boolean | undefined
  hidden: boolean
  deleting: boolean
  onToggle: () => void
  onDelete: () => void
  onDirtyChange: (key: string, dirty: boolean) => void
}) {
  const { t, i18n } = useTranslation()
  return (
    <article hidden={hidden} className='border-b last:border-b-0'>
      <div className='flex items-center gap-2 px-3 py-2.5'>
        <span className='bg-muted flex size-7 shrink-0 items-center justify-center rounded-md'>
          {credential.kind === 'none' ? (
            <Unplug className='text-muted-foreground size-4' />
          ) : (
            <KeyRound className='text-muted-foreground size-4' />
          )}
        </span>
        <div className='min-w-0 flex-1'>
          <h3
            className='line-clamp-2 text-sm font-medium break-all'
            title={credential.name}
          >
            {credential.name}
          </h3>
          <div className='text-muted-foreground mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs'>
            <span>{credentialKindLabel(credential.kind)}</span>
            {credential.expiresAt ? (
              <span>
                {t('channels.upstream.expires', {
                  value: formatDateTime(
                    credential.expiresAt,
                    toBcp47(i18n.language)
                  ),
                })}
              </span>
            ) : null}
            {credential.canRefresh && (
              <span>{t('channels.upstream.refreshable')}</span>
            )}
          </div>
        </div>
        <Badge variant={credential.enabled ? 'success' : 'secondary'}>
          {t(
            credential.enabled
              ? 'channels.imported.enabled'
              : 'channels.imported.disabled'
          )}
        </Badge>
        <Button
          type='button'
          variant='ghost'
          size='icon-sm'
          aria-label={t('channels.upstream.editCredential', {
            name: credential.name,
          })}
          aria-expanded={expanded ?? false}
          onClick={onToggle}
        >
          <Pencil className='size-4' />
        </Button>
        <Button
          type='button'
          variant='ghost'
          size='icon-sm'
          aria-label={t('channels.catalog.deleteCredential', {
            name: credential.name,
          })}
          disabled={deleting}
          onClick={onDelete}
        >
          <Trash2 className='size-4' />
        </Button>
      </div>
      {/* Once opened, the form keeps its draft through collapse and filtering. */}
      <div hidden={!expanded}>
        {expanded !== undefined && (
          <CredentialForm
            credential={credential}
            onDirtyChange={onDirtyChange}
          />
        )}
      </div>
    </article>
  )
}
