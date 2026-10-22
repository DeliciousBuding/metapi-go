import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useDirtyDialogClose } from '@/components/form/dirty-dialog-close'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Notice } from '@/components/ui/notice'
import { Switch } from '@/components/ui/switch'
import { gatewayUsersApi, type GatewayUserGroup } from '@/lib/api/gateway-users'

import { useAdminWrite, userAdminKeys } from './api'
import { PricingEditor, type EditorState } from './editors'
import { LoadError, Loading, WriteError } from './feedback'

export function GroupAccessDialog({
  group,
  close,
}: {
  group: GatewayUserGroup
  close: () => void
}) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const [grantedOnly, setGrantedOnly] = useState(false)
  const [pricingState, setPricingState] = useState<EditorState>({
    dirty: false,
    pending: false,
  })
  const { handleOpenChange, guard } = useDirtyDialogClose({
    enabled: pricingState.dirty,
    onOpenChange: (open) => {
      if (!open) close()
    },
  })
  const grants = useQuery({
    queryKey: userAdminKeys.grants(group.id),
    queryFn: () => gatewayUsersApi.grants(group.id),
  })
  const routes = useQuery({
    queryKey: userAdminKeys.routes,
    queryFn: gatewayUsersApi.publicRoutes,
  })
  const pricing = useQuery({
    queryKey: userAdminKeys.pricing(group.id),
    queryFn: () => gatewayUsersApi.pricing(group.id),
  })
  const mutation = useAdminWrite(
    ({ routeId, granted }: { routeId: number; granted: boolean }, token) =>
      granted
        ? gatewayUsersApi.revoke(group.id, routeId, token)
        : gatewayUsersApi.grant(group.id, routeId, token)
  )
  const grantedIds = new Set(grants.data?.items.map((item) => item.routeId))
  const unknownIds =
    grants.data?.items.filter(
      (item) => !routes.data?.some((route) => route.id === item.routeId)
    ) ?? []
  const ready =
    grants.isSuccess && !grants.isError && routes.isSuccess && !routes.isError
  const filteredRoutes =
    routes.data?.filter(
      (route) =>
        (!grantedOnly || grantedIds.has(route.id)) &&
        `${route.id} ${route.modelPattern} ${route.displayName ?? ''}`
          .toLowerCase()
          .includes(search.trim().toLowerCase())
    ) ?? []

  return (
    <>
      <Dialog
        open
        onOpenChange={(open) => {
          if (!mutation.isPending && !pricingState.pending)
            handleOpenChange(open)
        }}
      >
        <DialogContent className='max-h-[90svh] overflow-y-auto sm:max-w-2xl'>
          <DialogHeader>
            <DialogTitle>
              {t('userAdmin.accessTitle', { group: group.name })}
            </DialogTitle>
            <DialogDescription>
              {t('userAdmin.routeBoundary')}
            </DialogDescription>
          </DialogHeader>
          {!group.enabled && (
            <Notice tone='warning'>{t('userAdmin.groupDisabledHint')}</Notice>
          )}
          <section className='space-y-3'>
            <h2 className='text-base font-semibold'>
              {t('userAdmin.pricing')}
            </h2>
            {pricing.isPending && <Loading />}
            {pricing.isError && (
              <LoadError
                error={pricing.error}
                retry={() => void pricing.refetch()}
                pending={pricing.isFetching}
              />
            )}
            {pricing.isSuccess && (
              <PricingEditor
                groupId={group.id}
                pricing={pricing.data}
                onStateChange={setPricingState}
              />
            )}
          </section>
          <section className='space-y-3 border-t pt-4'>
            <h2 className='text-base font-semibold'>{t('userAdmin.routes')}</h2>
            {grants.isSuccess && !grants.isError && (
              <p className='text-muted-foreground text-xs'>
                {t('userAdmin.grantsCount', {
                  count: grants.data?.items.length ?? 0,
                })}
              </p>
            )}
            {(grants.isPending || routes.isPending) && <Loading />}
            <LoadError
              error={grants.error}
              retry={() => void grants.refetch()}
              pending={grants.isFetching}
            />
            <LoadError
              error={routes.error}
              retry={() => void routes.refetch()}
              pending={routes.isFetching}
            />
            <WriteError error={mutation.error} />
            {ready && (
              <>
                <div className='flex flex-wrap items-center gap-3'>
                  <Input
                    className='h-8 min-w-0 flex-1 basis-48'
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                    maxLength={200}
                    aria-label={t('userAdmin.routeSearch')}
                    placeholder={t('userAdmin.routeSearch')}
                  />
                  <div className='flex items-center gap-2'>
                    <Switch
                      id={'granted-only-' + group.id}
                      checked={grantedOnly}
                      onCheckedChange={setGrantedOnly}
                    />
                    <Label htmlFor={'granted-only-' + group.id}>
                      {t('userAdmin.grantedOnly')}
                    </Label>
                  </div>
                </div>
                <p className='text-muted-foreground text-xs'>
                  {t('userAdmin.routeMatches', {
                    count: filteredRoutes.length,
                    total: routes.data.length,
                  })}
                </p>
                <div className='max-h-80 divide-y overflow-y-auto rounded-lg border'>
                  {filteredRoutes.length === 0 ? (
                    <p className='text-muted-foreground p-4'>
                      {t(
                        routes.data.length === 0
                          ? 'userAdmin.noRoutes'
                          : 'userAdmin.noMatches'
                      )}
                    </p>
                  ) : (
                    filteredRoutes.map((route) => {
                      const granted = grantedIds.has(route.id)
                      return (
                        <div
                          key={route.id}
                          className='flex flex-wrap items-center justify-between gap-3 p-3'
                        >
                          <div className='min-w-0 flex-1'>
                            <p className='font-medium wrap-anywhere'>
                              {route.displayName || route.modelPattern}
                            </p>
                            <p className='text-muted-foreground text-xs wrap-anywhere'>
                              #{route.id} · {route.modelPattern} ·{' '}
                              {t(
                                route.enabled
                                  ? 'userAdmin.enabled'
                                  : 'userAdmin.disabled'
                              )}
                            </p>
                          </div>
                          <Button
                            size='sm'
                            variant={granted ? 'outline' : 'secondary'}
                            disabled={
                              mutation.isPending ||
                              grants.isFetching ||
                              routes.isFetching
                            }
                            aria-label={t(
                              granted
                                ? 'userAdmin.revokeNamed'
                                : 'userAdmin.grantNamed',
                              { route: route.displayName || route.modelPattern }
                            )}
                            onClick={() =>
                              mutation.submit({ routeId: route.id, granted })
                            }
                          >
                            {t(
                              granted ? 'userAdmin.revoke' : 'userAdmin.grant'
                            )}
                          </Button>
                        </div>
                      )
                    })
                  )}
                </div>
                {unknownIds.map((grant) => (
                  <div
                    key={grant.routeId}
                    className='flex items-center justify-between gap-3'
                  >
                    <span>
                      {t('userAdmin.unavailableRoute', { id: grant.routeId })}
                    </span>
                    <Button
                      size='sm'
                      variant='outline'
                      disabled={
                        mutation.isPending ||
                        grants.isFetching ||
                        routes.isFetching
                      }
                      onClick={() =>
                        mutation.submit({
                          routeId: grant.routeId,
                          granted: true,
                        })
                      }
                    >
                      {t('userAdmin.revoke')}
                    </Button>
                  </div>
                ))}
                {grantedIds.size === 0 && (
                  <Notice tone='warning'>{t('userAdmin.emptyGrants')}</Notice>
                )}
              </>
            )}
          </section>
          <div className='flex justify-end'>
            <Button
              variant='outline'
              onClick={() => handleOpenChange(false)}
              disabled={mutation.isPending || pricingState.pending}
            >
              {t('userAdmin.close')}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
      {guard}
    </>
  )
}
