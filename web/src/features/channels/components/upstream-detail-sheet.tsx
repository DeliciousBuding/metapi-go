import { useQuery } from '@tanstack/react-query'
import { Cable, KeyRound, Route, Boxes, Trash2 } from 'lucide-react'
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { api } from '@/lib/api'
import type {
  ImportedMember,
  ImportedUpstream,
} from '@/lib/api/imported-upstreams'

import { upstreamKeys } from '../lib/upstream-config'
import { UpstreamConnectionForm } from './upstream-connection-form'
import { UpstreamCredentials } from './upstream-credentials'
import { UpstreamIdentity } from './upstream-identity'
import { UpstreamModels } from './upstream-models'
import { UpstreamRouteBindings } from './upstream-route-bindings'

export function UpstreamDetailSheet(props: {
  item: ImportedUpstream
  members: ImportedMember[]
  onClose: () => void
  onDelete?: () => void
  deleting?: boolean
}) {
  const { t } = useTranslation()
  const [tab, setTab] = useState('connection')
  const [dirtyForms, setDirtyForms] = useState<Record<string, boolean>>({})
  const [pendingDelete, setPendingDelete] = useState<(() => void) | null>(null)
  const dirty = Object.values(dirtyForms).some(Boolean)
  function beforeDelete(action: () => void) {
    if (dirty) setPendingDelete(() => action)
    else action()
  }
  const onDirtyChange = useCallback(
    (key: string, dirty: boolean) =>
      setDirtyForms((previous) =>
        previous[key] === dirty ? previous : { ...previous, [key]: dirty }
      ),
    []
  )
  const query = useQuery({
    queryKey: upstreamKeys.detail(props.item.id),
    queryFn: () => api.getImportedUpstream(props.item.id),
  })
  const { handleOpenChange, guard } = useDirtyDialogClose({
    enabled: dirty,
    onOpenChange: (open) => {
      if (!open) props.onClose()
    },
  })
  return (
    <>
      <Sheet open onOpenChange={handleOpenChange}>
        <SheetContent
          className='flex w-full flex-col gap-0 p-0 sm:max-w-2xl'
          showMobileCloseBar={false}
        >
          <SheetHeader className='shrink-0 space-y-3 border-b p-5 pr-12'>
            <UpstreamIdentity
              provider={props.item.provider}
              dialect={props.item.dialect}
            />
            <SheetTitle className='text-xl'>{props.item.name}</SheetTitle>
            <SheetDescription className='truncate text-xs'>
              {props.item.baseUrl}
            </SheetDescription>
          </SheetHeader>
          <Tabs
            value={tab}
            onValueChange={(value) => setTab(String(value))}
            className='min-h-0 flex-1 gap-0'
          >
            <TabsList
              variant='line'
              className='mx-5 my-3 w-auto shrink-0 justify-between gap-1'
            >
              <TabsTrigger value='connection'>
                <Cable />
                {t('channels.upstream.connection')}
              </TabsTrigger>
              <TabsTrigger value='credentials'>
                <KeyRound />
                {t('channels.upstream.credentials')}
              </TabsTrigger>
              <TabsTrigger value='models'>
                <Boxes />
                {t('channels.catalog.models')}
              </TabsTrigger>
              <TabsTrigger value='routes'>
                <Route />
                {t('channels.upstream.routes')}
              </TabsTrigger>
            </TabsList>
            <div className='min-h-0 flex-1 overflow-y-auto px-5 pt-2 pb-5'>
              <TabsContent value='connection' keepMounted>
                {query.error && (
                  <QueryErrorBanner
                    error={query.error}
                    messageKey='channels.imported.loadError'
                    onRetry={() => query.refetch()}
                    isRetrying={query.isFetching}
                  />
                )}
                {query.isLoading && (
                  <p className='text-muted-foreground py-8 text-center'>
                    {t('channels.upstream.loading')}
                  </p>
                )}
                {query.data && (
                  <UpstreamConnectionForm
                    detail={query.data}
                    onDirtyChange={onDirtyChange}
                  />
                )}
              </TabsContent>
              <TabsContent value='models' keepMounted>
                {query.data && (
                  <UpstreamModels
                    detail={query.data}
                    active={tab === 'models'}
                    members={props.members}
                    onDirtyChange={onDirtyChange}
                    beforeDelete={beforeDelete}
                  />
                )}
              </TabsContent>
              <TabsContent value='credentials' keepMounted>
                <UpstreamCredentials
                  id={props.item.id}
                  active={tab === 'credentials'}
                  onDirtyChange={onDirtyChange}
                  beforeDelete={beforeDelete}
                />
              </TabsContent>
              <TabsContent value='routes' keepMounted>
                <UpstreamRouteBindings
                  channelId={props.item.id}
                  active={tab === 'routes'}
                  beforeDelete={beforeDelete}
                  members={props.members}
                  onDirtyChange={onDirtyChange}
                />
              </TabsContent>
            </div>
          </Tabs>
          {props.onDelete && (
            <div className='border-t px-5 py-3'>
              <Button
                variant='ghost'
                size='sm'
                className='text-destructive'
                disabled={props.deleting}
                onClick={() => props.onDelete && beforeDelete(props.onDelete)}
              >
                <Trash2 className='size-4' />
                {t('channels.catalog.deleteChannel')}
              </Button>
            </div>
          )}
        </SheetContent>
      </Sheet>
      {guard}
      <ConfirmDialog
        open={!!pendingDelete}
        title={t('settings.common.unsavedTitle')}
        description={t('settings.common.unsavedDescription')}
        confirmLabel={t('settings.common.discardChanges')}
        cancelLabel={t('settings.common.keepEditing')}
        destructive
        onCancel={() => setPendingDelete(null)}
        onConfirm={() => {
          const action = pendingDelete
          setPendingDelete(null)
          action?.()
        }}
      />
    </>
  )
}
