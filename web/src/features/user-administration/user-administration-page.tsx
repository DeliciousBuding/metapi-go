import { useQuery } from '@tanstack/react-query'
import { useSearch } from '@tanstack/react-router'
import { Plus, RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { PageHeader } from '@/components/common/page-header'
import { QuietRefreshStatus } from '@/components/common/quiet-refresh-status'
import { Button } from '@/components/ui/button'
import { Notice } from '@/components/ui/notice'
import { useDelayedSpin } from '@/hooks/use-quiet-refresh'
import {
  gatewayUsersApi,
  type GatewayUser,
  type GatewayUserGroup,
} from '@/lib/api/gateway-users'

import { useAdminWrite, userAdminKeys } from './api'
import { LoadError, Loading, WriteError } from './feedback'
import { useUserDirectoryDialogs } from './use-user-directory-dialogs'
import { UserDirectoryFilters } from './user-directory-filters'
import { UserRow, GroupRow } from './user-directory-rows'

export function UserAdministrationPage() {
  const { t } = useTranslation()
  const search = useSearch({ from: '/_authenticated/users' })
  const view = search.view ?? 'users'
  const users = useQuery({
    queryKey: userAdminKeys.users,
    queryFn: gatewayUsersApi.users,
  })
  const groups = useQuery({
    queryKey: userAdminKeys.groups,
    queryFn: gatewayUsersApi.groups,
  })
  const { actions, dialogs } = useUserDirectoryDialogs(groups.data?.items ?? [])
  const toggle = useAdminWrite(
    (
      {
        kind,
        item,
      }:
        | { kind: 'user'; item: GatewayUser }
        | { kind: 'group'; item: GatewayUserGroup },
      token
    ) =>
      kind === 'user'
        ? gatewayUsersApi.updateUser(
            item.id,
            {
              displayName: item.displayName,
              groupId: item.groupId,
              enabled: !item.enabled,
            },
            token
          )
        : gatewayUsersApi.updateGroup(
            item.id,
            { name: item.name, enabled: !item.enabled },
            token
          )
  )
  const query = view === 'users' ? users : groups
  const filter = (item: { enabled: boolean }, text: string) =>
    (!search.status || item.enabled === (search.status === 'enabled')) &&
    text.toLowerCase().includes((search.q ?? '').trim().toLowerCase())
  const filteredUsers =
    users.data?.items.filter((user) =>
      filter(
        user,
        `${user.username} ${user.displayName} ${groups.data?.items.find((group) => group.id === user.groupId)?.name ?? ''}`
      )
    ) ?? []
  const filteredGroups =
    groups.data?.items.filter((group) =>
      filter(group, `${group.name} ${group.slug}`)
    ) ?? []
  const count = view === 'users' ? filteredUsers.length : filteredGroups.length
  let emptyKey = 'userAdmin.noMatches'
  if (!search.q && !search.status)
    emptyKey = view === 'users' ? 'userAdmin.noUsers' : 'userAdmin.noGroups'
  const isRefreshing = users.isFetching || groups.isFetching
  const showRefreshSpin = useDelayedSpin(isRefreshing)
  const updatedAt = Math.max(
    users.dataUpdatedAt ?? 0,
    groups.dataUpdatedAt ?? 0
  )
  const hasRefreshError = Boolean(users.isError || groups.isError)

  return (
    <div className='page-padding flex h-full min-w-0 flex-col gap-3 overflow-y-auto'>
      <PageHeader
        title={t('userAdmin.title')}
        description={t('userAdmin.description')}
        actions={
          <>
            <QuietRefreshStatus
              updatedAt={updatedAt}
              hasError={hasRefreshError}
              className='hidden sm:inline'
            />
            <Button
              size='sm'
              variant='outline'
              disabled={isRefreshing}
              onClick={() => {
                void users.refetch()
                void groups.refetch()
              }}
            >
              <RefreshCw
                aria-hidden='true'
                className={showRefreshSpin ? 'animate-spin' : undefined}
              />
              {t('userAdmin.refresh')}
            </Button>
            <Button
              size='sm'
              disabled={view === 'users' && !groups.isSuccess}
              onClick={() =>
                actions.edit({ kind: view === 'users' ? 'user' : 'group' })
              }
            >
              <Plus aria-hidden='true' />
              {t(
                view === 'users'
                  ? 'userAdmin.createUser'
                  : 'userAdmin.createGroup'
              )}
            </Button>
          </>
        }
      />
      <UserDirectoryFilters />
      {query.isPending && <Loading />}
      <LoadError
        error={query.error}
        retry={() => void query.refetch()}
        pending={query.isFetching}
      />
      {view === 'users' && (
        <LoadError
          error={groups.error}
          retry={() => void groups.refetch()}
          pending={groups.isFetching}
        />
      )}
      <WriteError error={toggle.error} />
      {query.isSuccess && !query.isError && (
        <>
          <p className='text-muted-foreground text-xs tabular-nums'>
            {t('userAdmin.directoryCount', { count, limit: query.data.limit })}
          </p>
          {query.data.items.length >= query.data.limit && (
            <Notice tone='warning'>
              {t('userAdmin.directoryLimit', { limit: query.data.limit })}
            </Notice>
          )}
          {count === 0 ? (
            <div className='rounded-xl border border-dashed p-6 text-center'>
              <p>{t(emptyKey)}</p>
              <Button
                variant='outline'
                size='sm'
                className='mt-3'
                onClick={() => void query.refetch()}
              >
                {t('userAdmin.refresh')}
              </Button>
            </div>
          ) : (
            <div className='bg-card min-w-0 divide-y rounded-xl border'>
              {view === 'users'
                ? filteredUsers.map((user) => (
                    <UserRow
                      key={user.id}
                      user={user}
                      userGroup={groups.data?.items.find(
                        (group) => group.id === user.groupId
                      )}
                      actions={actions}
                      editDisabled={!groups.isSuccess || groups.isError}
                      toggleDisabled={toggle.isPending || users.isFetching}
                      onToggle={() =>
                        toggle.submit({ kind: 'user', item: user })
                      }
                    />
                  ))
                : filteredGroups.map((group) => (
                    <GroupRow
                      key={group.id}
                      group={group}
                      actions={actions}
                      toggleDisabled={toggle.isPending || groups.isFetching}
                      onToggle={() =>
                        toggle.submit({ kind: 'group', item: group })
                      }
                    />
                  ))}
            </div>
          )}
        </>
      )}
      <p className='text-muted-foreground text-xs'>
        {t(
          view === 'users'
            ? 'userAdmin.userBoundary'
            : 'userAdmin.groupBoundary'
        )}
      </p>
      {dialogs}
    </div>
  )
}
