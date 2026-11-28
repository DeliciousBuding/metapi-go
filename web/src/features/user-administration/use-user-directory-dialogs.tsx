import { useState } from 'react'

import type { GatewayUser, GatewayUserGroup } from '@/lib/api/gateway-users'

import { ActivationDialog, BudgetDialog, IdentityEditor } from './editors'
import { GroupAccessDialog } from './group-access'
import { ReadinessDialog } from './readiness'

type Editor =
  | { kind: 'user'; user?: GatewayUser }
  | { kind: 'group'; group?: GatewayUserGroup }

export function useUserDirectoryDialogs(groups: GatewayUserGroup[]) {
  const [editor, setEditor] = useState<Editor | null>(null)
  const [budgetUser, setBudgetUser] = useState<GatewayUser | null>(null)
  const [activationUser, setActivationUser] = useState<GatewayUser | null>(null)
  const [accessGroup, setAccessGroup] = useState<GatewayUserGroup | null>(null)
  const [readinessUserId, setReadinessUserId] = useState<number | null>(null)
  const [resumeReadiness, setResumeReadiness] = useState<number | null>(null)
  const closeTask = () => {
    if (resumeReadiness !== null) {
      setReadinessUserId(resumeReadiness)
      setResumeReadiness(null)
    }
  }
  const dialogs = (
    <>
      {editor && (
        <IdentityEditor
          {...editor}
          groups={groups}
          close={() => {
            setEditor(null)
            closeTask()
          }}
        />
      )}
      {budgetUser && (
        <BudgetDialog
          user={budgetUser}
          close={() => {
            setBudgetUser(null)
            closeTask()
          }}
        />
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
          close={() => {
            setAccessGroup(null)
            closeTask()
          }}
        />
      )}
      {readinessUserId !== null && (
        <ReadinessDialog
          userId={readinessUserId}
          close={() => setReadinessUserId(null)}
          editUser={(user) => {
            setResumeReadiness(readinessUserId)
            setEditor({ kind: 'user', user })
          }}
          editGroup={(group) => {
            setResumeReadiness(readinessUserId)
            setEditor({ kind: 'group', group })
          }}
          editAccess={(group) => {
            setResumeReadiness(readinessUserId)
            setAccessGroup(group)
          }}
          editBudget={(user) => {
            setResumeReadiness(readinessUserId)
            setBudgetUser(user)
          }}
        />
      )}
    </>
  )
  return {
    actions: {
      edit: setEditor,
      openBudget: setBudgetUser,
      openActivation: setActivationUser,
      openAccess: setAccessGroup,
      openReadiness: setReadinessUserId,
    },
    dialogs,
  }
}
export type DirectoryActions = ReturnType<
  typeof useUserDirectoryDialogs
>['actions']
