import { useQuery } from '@tanstack/react-query'
import { useNavigate, useSearch } from '@tanstack/react-router'
import { Plus, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PageHeader } from '@/components/common/page-header'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Notice } from '@/components/ui/notice'
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '@/components/ui/select'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  gatewayUsersApi,
  type GatewayUser,
  type GatewayUserGroup,
} from '@/lib/api/gateway-users'

import { useAdminWrite, userAdminKeys } from './api'
import { ActivationDialog, BudgetDialog, IdentityEditor } from './editors'
import { LoadError, Loading, WriteError } from './feedback'
import { GroupAccessDialog } from './group-access'
import { ReadinessDialog } from './readiness'

type Editor =
  | { kind: 'user'; user?: GatewayUser }
  | { kind: 'group'; group?: GatewayUserGroup }

export function UserAdministrationPage() {
  const { t } = useTranslation()
  const search = useSearch({ from: '/_authenticated/users' })
  const navigate = useNavigate({ from: '/users' })
  const view = search.view ?? 'users'
  const users = useQuery({
    queryKey: userAdminKeys.users,
    queryFn: gatewayUsersApi.users,
  })
  const groups = useQuery({
    queryKey: userAdminKeys.groups,
    queryFn: gatewayUsersApi.groups,
  })
  const [editor, setEditor] = useState<Editor | null>(null)
  const [budgetUser, setBudgetUser] = useState<GatewayUser | null>(null)
  const [activationUser, setActivationUser] = useState<GatewayUser | null>(null)
  const [accessGroup, setAccessGroup] = useState<GatewayUserGroup | null>(null)
  const [readinessUserId, setReadinessUserId] = useState<number | null>(null)
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
  const updateSearch = (next: Partial<typeof search>, replace = false) => {
    void navigate({
      search: (previous) => ({ ...previous, ...next }),
      replace,
    })
  }

  return (
    <div className='page-padding flex h-full flex-col gap-4 overflow-y-auto'>
      <PageHeader
        title={t('userAdmin.title')}
        description={t('userAdmin.description')}
        actions={
          <>
            <Button
              size='sm'
              variant='outline'
              disabled={users.isFetching || groups.isFetching}
              onClick={() => {
                void users.refetch()
                void groups.refetch()
              }}
            >
              <RefreshCw aria-hidden='true' />
              {t('userAdmin.refresh')}
            </Button>
            <Button
              size='sm'
              disabled={view === 'users' && !groups.isSuccess}
              onClick={() =>
                setEditor({ kind: view === 'users' ? 'user' : 'group' })
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
      <Tabs
        value={view}
        onValueChange={(value) =>
          updateSearch({
            view: value as 'users' | 'groups',
            q: undefined,
            status: undefined,
          })
        }
      >
        <TabsList aria-label={t('userAdmin.views')}>
          <TabsTrigger value='users'>{t('userAdmin.users')}</TabsTrigger>
          <TabsTrigger value='groups'>{t('userAdmin.groups')}</TabsTrigger>
        </TabsList>
      </Tabs>
      <div className='flex flex-wrap items-center gap-2'>
        <Input
          className='h-8 w-full sm:w-72'
          aria-label={t('userAdmin.search')}
          placeholder={t('userAdmin.search')}
          value={search.q ?? ''}
          onChange={(event) =>
            updateSearch({ q: event.target.value || undefined }, true)
          }
        />
        <Select
          value={search.status ?? 'all'}
          onValueChange={(value) =>
            updateSearch({
              status:
                value === 'all' ? undefined : (value as 'enabled' | 'disabled'),
            })
          }
        >
          <SelectTrigger size='sm' aria-label={t('userAdmin.status')}>
            <SelectValue>
              {t(
                search.status
                  ? `userAdmin.${search.status}`
                  : 'userAdmin.allStatuses'
              )}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>{t('userAdmin.allStatuses')}</SelectItem>
            <SelectItem value='enabled'>{t('userAdmin.enabled')}</SelectItem>
            <SelectItem value='disabled'>{t('userAdmin.disabled')}</SelectItem>
          </SelectContent>
        </Select>
        {(search.q || search.status) && (
          <Button
            size='sm'
            variant='ghost'
            onClick={() => updateSearch({ q: undefined, status: undefined })}
          >
            {t('userAdmin.reset')}
          </Button>
        )}
      </div>
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
          <p className='text-muted-foreground text-xs'>
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
            <div className='bg-card divide-y rounded-xl border'>
              {view === 'users'
                ? filteredUsers.map((user) => (
                    <article
                      key={user.id}
                      className='flex flex-wrap items-center justify-between gap-3 p-4'
                    >
                      <div className='min-w-0 flex-1 basis-56'>
                        <div className='flex flex-wrap items-center gap-2'>
                          <h2 className='text-sm font-semibold wrap-anywhere'>
                            {user.displayName || user.username}
                          </h2>
                          <span className='bg-muted rounded px-2 py-0.5 text-xs'>
                            {t(
                              user.enabled
                                ? 'userAdmin.enabled'
                                : 'userAdmin.disabled'
                            )}
                          </span>
                        </div>
                        <p className='text-muted-foreground mt-1 text-xs wrap-anywhere'>
                          {user.username} · #{user.id} · {t('userAdmin.group')}:{' '}
                          {groups.data?.items.find(
                            (group) => group.id === user.groupId
                          )?.name ?? `#${user.groupId}`}
                        </p>
                      </div>
                      <div className='flex flex-wrap gap-2'>
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() => setReadinessUserId(user.id)}
                        >
                          {t('userAdmin.readiness.open')}
                        </Button>
                        <Button
                          size='sm'
                          variant='outline'
                          disabled={!groups.isSuccess || groups.isError}
                          onClick={() => setEditor({ kind: 'user', user })}
                        >
                          {t('userAdmin.editUser')}
                        </Button>
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() => setBudgetUser(user)}
                        >
                          {t('userAdmin.budget')}
                        </Button>
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() => setActivationUser(user)}
                        >
                          {t('userAdmin.activation')}
                        </Button>
                        <Button
                          size='sm'
                          variant='ghost'
                          disabled={toggle.isPending || users.isFetching}
                          onClick={() =>
                            toggle.submit({ kind: 'user', item: user })
                          }
                        >
                          {t(
                            user.enabled
                              ? 'userAdmin.disable'
                              : 'userAdmin.enable'
                          )}
                        </Button>
                      </div>
                    </article>
                  ))
                : filteredGroups.map((group) => (
                    <article
                      key={group.id}
                      className='flex flex-wrap items-center justify-between gap-3 p-4'
                    >
                      <div className='min-w-0 flex-1 basis-56'>
                        <div className='flex flex-wrap items-center gap-2'>
                          <h2 className='text-sm font-semibold wrap-anywhere'>
                            {group.name}
                          </h2>
                          <span className='bg-muted rounded px-2 py-0.5 text-xs'>
                            {t(
                              group.enabled
                                ? 'userAdmin.enabled'
                                : 'userAdmin.disabled'
                            )}
                          </span>
                        </div>
                        <p className='text-muted-foreground mt-1 text-xs wrap-anywhere'>
                          {group.slug} · #{group.id}
                        </p>
                      </div>
                      <div className='flex flex-wrap gap-2'>
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() => setEditor({ kind: 'group', group })}
                        >
                          {t('userAdmin.editGroup')}
                        </Button>
                        <Button
                          size='sm'
                          variant='outline'
                          onClick={() => setAccessGroup(group)}
                        >
                          {t('userAdmin.accessAndPricing')}
                        </Button>
                        <Button
                          size='sm'
                          variant='ghost'
                          disabled={toggle.isPending || groups.isFetching}
                          onClick={() =>
                            toggle.submit({ kind: 'group', item: group })
                          }
                        >
                          {t(
                            group.enabled
                              ? 'userAdmin.disable'
                              : 'userAdmin.enable'
                          )}
                        </Button>
                      </div>
                    </article>
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
      {editor && (
        <IdentityEditor
          {...editor}
          groups={groups.data?.items ?? []}
          close={() => setEditor(null)}
        />
      )}
      {budgetUser && (
        <BudgetDialog user={budgetUser} close={() => setBudgetUser(null)} />
      )}
      {activationUser && (
        <ActivationDialog
          user={activationUser}
          close={() => setActivationUser(null)}
        />
      )}
      {accessGroup && (
        <GroupAccessDialog
          group={accessGroup}
          close={() => setAccessGroup(null)}
        />
      )}
      {readinessUserId !== null && (
        <ReadinessDialog
          userId={readinessUserId}
          close={() => setReadinessUserId(null)}
          editUser={(user) => setEditor({ kind: 'user', user })}
          editGroup={(group) => setEditor({ kind: 'group', group })}
          editAccess={setAccessGroup}
          editBudget={setBudgetUser}
        />
      )}
    </div>
  )
}
