import { useTranslation } from 'react-i18next'

import { SoftToneBadge } from '@/components/common/soft-tone-badge'
import { Button } from '@/components/ui/button'
import type { GatewayUser, GatewayUserGroup } from '@/lib/api/gateway-users'

import type { DirectoryActions } from './use-user-directory-dialogs'

export function UserRow({
  user,
  userGroup,
  actions,
  editDisabled,
  toggleDisabled,
  onToggle,
}: {
  user: GatewayUser
  userGroup?: GatewayUserGroup
  actions: DirectoryActions
  editDisabled: boolean
  toggleDisabled: boolean
  onToggle: () => void
}) {
  const { t } = useTranslation()
  return (
    <article className='flex min-w-0 flex-wrap items-center justify-between gap-3 px-4 py-3'>
      <div className='min-w-0 flex-1 basis-52'>
        <div className='flex flex-wrap items-center gap-2'>
          <h2 className='text-sm font-semibold wrap-anywhere'>
            {user.displayName || user.username}
          </h2>
          <SoftToneBadge tone={user.enabled ? 'success' : 'neutral'}>
            {t(user.enabled ? 'userAdmin.enabled' : 'userAdmin.disabled')}
          </SoftToneBadge>
          {userGroup && (
            <SoftToneBadge
              tone={userGroup.enabled ? 'info' : 'warning'}
              showDot={false}
            >
              <span className='max-w-40 truncate'>{userGroup.name}</span>
            </SoftToneBadge>
          )}
        </div>
        <p className='text-muted-foreground mt-1 text-xs wrap-anywhere tabular-nums'>
          {user.username} · #{user.id} · {t('userAdmin.group')}:{' '}
          {userGroup?.name ?? `#${user.groupId}`}
        </p>
      </div>
      <div className='flex min-w-0 flex-wrap gap-1.5'>
        <Button
          size='sm'
          variant='outline'
          onClick={() => actions.openReadiness(user.id)}
        >
          {t('userAdmin.readiness.open')}
        </Button>
        <Button
          size='sm'
          variant='outline'
          disabled={editDisabled}
          onClick={() => actions.edit({ kind: 'user', user })}
        >
          {t('userAdmin.editUser')}
        </Button>
        <Button
          size='sm'
          variant='outline'
          onClick={() => actions.openBudget(user)}
        >
          {t('userAdmin.budget')}
        </Button>
        <Button
          size='sm'
          variant='outline'
          onClick={() => actions.openActivation(user)}
        >
          {t('userAdmin.activation')}
        </Button>
        <Button
          size='sm'
          variant='ghost'
          disabled={toggleDisabled}
          onClick={onToggle}
        >
          {t(user.enabled ? 'userAdmin.disable' : 'userAdmin.enable')}
        </Button>
      </div>
    </article>
  )
}
export function GroupRow({
  group,
  actions,
  toggleDisabled,
  onToggle,
}: {
  group: GatewayUserGroup
  actions: DirectoryActions
  toggleDisabled: boolean
  onToggle: () => void
}) {
  const { t } = useTranslation()
  return (
    <article className='flex min-w-0 flex-wrap items-center justify-between gap-3 px-4 py-3'>
      <div className='min-w-0 flex-1 basis-52'>
        <div className='flex flex-wrap items-center gap-2'>
          <h2 className='text-sm font-semibold wrap-anywhere'>{group.name}</h2>
          <SoftToneBadge tone={group.enabled ? 'success' : 'neutral'}>
            {t(group.enabled ? 'userAdmin.enabled' : 'userAdmin.disabled')}
          </SoftToneBadge>
        </div>
        <p className='text-muted-foreground mt-1 text-xs wrap-anywhere tabular-nums'>
          {group.slug} · #{group.id}
        </p>
      </div>
      <div className='flex min-w-0 flex-wrap gap-1.5'>
        <Button
          size='sm'
          variant='outline'
          onClick={() => actions.edit({ kind: 'group', group })}
        >
          {t('userAdmin.editGroup')}
        </Button>
        <Button
          size='sm'
          variant='outline'
          onClick={() => actions.openAccess(group)}
        >
          {t('userAdmin.accessAndPricing')}
        </Button>
        <Button
          size='sm'
          variant='ghost'
          disabled={toggleDisabled}
          onClick={onToggle}
        >
          {t(group.enabled ? 'userAdmin.disable' : 'userAdmin.enable')}
        </Button>
      </div>
    </article>
  )
}
