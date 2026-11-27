// Unlocks a downstream key for verification, client imports and config export.
// Exports live for this dialog session; reopening requires confirmation again.

import { Link2, Lock } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { CredentialExportProfileId as ExportProfileId } from '@/lib/api/credential-export'

import { CredentialClientSetup } from './credential-client-setup'
import { CredentialExportEndpoint } from './credential-export-endpoint'
import {
  getClientContext,
  type CredentialExportTarget,
} from './credential-export-profile'
import { CredentialVerifySection } from './credential-verify-section'
import { useCredentialExportSession } from './use-credential-export-session'

export type { CredentialExportTarget } from './credential-export-profile'

interface CredentialExportDialogProps {
  target: CredentialExportTarget | null
  onOpenChange: (open: boolean) => void
}

export function CredentialExportDialog({
  target,
  onOpenChange,
}: CredentialExportDialogProps) {
  return target ? (
    <OpenCredentialExportDialog
      key={target.id}
      target={target}
      onOpenChange={onOpenChange}
    />
  ) : null
}

function OpenCredentialExportDialog({
  target,
  onOpenChange,
}: {
  target: CredentialExportTarget
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const session = useCredentialExportSession(target.id)
  const { client, model, data, phase } = session

  function handleOpenChange(open: boolean) {
    if (!open) session.close()
    onOpenChange(open)
  }

  function renderLocked() {
    return (
      <div className='space-y-3 rounded-lg border border-dashed px-4 py-6'>
        <div className='flex items-center gap-2'>
          <Lock className='text-muted-foreground size-4' />
          <p className='text-sm font-medium'>{t('connect.lockedTitle')}</p>
        </div>
        <p className='text-muted-foreground text-xs'>
          {t('connect.lockedDescription')}
        </p>
        <form
          className='space-y-2'
          onSubmit={(event) => {
            event.preventDefault()
            session.unlock()
          }}
        >
          <Input
            type='password'
            autoComplete='current-password'
            value={session.unlockDraft}
            onChange={(event) => session.setUnlockDraft(event.target.value)}
            placeholder={t('reauth.tokenPlaceholder')}
            aria-label={t('reauth.tokenLabel')}
          />
          {phase === 'rejected' && (
            <p role='alert' className='text-destructive-soft-fg text-xs'>
              {t('reauth.errorInvalid')}
            </p>
          )}
          <Button
            type='submit'
            size='sm'
            disabled={!session.unlockDraft.trim()}
          >
            {t('connect.unlock')}
          </Button>
        </form>
      </div>
    )
  }

  function renderBody() {
    if (phase === 'locked' || phase === 'rejected') {
      return renderLocked()
    }
    if (phase === 'loading' && !data) {
      return (
        <div className='space-y-3'>
          <Skeleton className='h-16 w-full rounded-lg' />
          <Skeleton className='h-24 w-full rounded-lg' />
        </div>
      )
    }
    if (phase === 'error' || !data) {
      return (
        <div className='rounded-lg border border-dashed px-4 py-6 text-center'>
          <p className='text-destructive-soft-fg text-sm'>
            {t('connect.loadFailed')}
          </p>
          <Button
            variant='outline'
            size='sm'
            className='mt-3'
            onClick={session.refresh}
          >
            {t('common.retry')}
          </Button>
        </div>
      )
    }
    const context = getClientContext(data, client)
    const { apiBase, apiKey, protocol } = context
    return (
      <div className='space-y-4'>
        <CredentialExportEndpoint
          clientBase={context.clientBase}
          apiKey={apiKey}
          keyMasked={target.keyMasked}
          copiedKey={session.copiedKey}
          showKey={session.showKey}
          onToggleKey={session.toggleKey}
          onCopyKey={() => void session.copyKey(apiKey)}
          onCopyEndpoint={() => session.copyEndpoint(context.clientBase)}
        />
        <div className='space-y-2'>
          <Label htmlFor='credential-client'>{t('connect.clientLabel')}</Label>
          <Select
            items={data.profiles.map((profile) => ({
              value: profile.id,
              label: profile.label,
            }))}
            value={client}
            onValueChange={(value) => {
              if (value) session.changeClient(value as ExportProfileId)
            }}
          >
            <SelectTrigger id='credential-client' className='w-full'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {data.profiles.map((profile) => (
                <SelectItem key={profile.id} value={profile.id}>
                  {profile.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className='text-muted-foreground text-xs break-all'>
            {t('connect.modelLabel')}: {model || t('connect.modelRequired')}
          </p>
        </div>

        <Tabs defaultValue='verify'>
          <TabsList className='w-full'>
            <TabsTrigger value='verify'>{t('connect.verifyTab')}</TabsTrigger>
            <TabsTrigger value='clients'>{t('connect.clientsTab')}</TabsTrigger>
          </TabsList>
          <TabsContent value='verify' keepMounted>
            {apiKey ? (
              <CredentialVerifySection
                keyId={target.id}
                apiBase={apiBase}
                apiKey={apiKey}
                contextId={client}
                model={model}
                protocol={protocol}
                onModelChange={session.changeModel}
                onProtocolChange={session.changeProtocol}
              />
            ) : null}
          </TabsContent>
          <TabsContent value='clients' className='space-y-4'>
            <CredentialClientSetup
              target={target}
              context={context}
              profiles={data.profiles}
              model={model}
              currentConfiguration={
                phase === 'ready' && !!model && data.model === model
              }
              loading={phase === 'loading'}
              showKey={session.showKey}
              pendingDeepLink={session.pendingDeepLink}
              setPendingDeepLink={session.setPendingDeepLink}
              changeClient={session.changeClient}
              onRefresh={session.refresh}
              onCopy={session.copyConfig}
            />
          </TabsContent>
        </Tabs>
      </div>
    )
  }

  return (
    <Dialog open onOpenChange={handleOpenChange}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2'>
            <Link2 className='text-primary size-4' />
            {t('connect.title', { name: target.name })}
          </DialogTitle>
          <DialogDescription>{t('connect.description')}</DialogDescription>
        </DialogHeader>

        {renderBody()}

        <DialogFooter>
          <Button variant='outline' onClick={() => handleOpenChange(false)}>
            {t('settings.common.cancel')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
