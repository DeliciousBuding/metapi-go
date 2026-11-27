import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import type { CredentialExportTarget } from '@/components/common/credential-export-dialog'
import { useReauthReplay } from '@/components/common/reauth-provider'
import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import {
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { api } from '@/lib/api'
import { toast } from '@/lib/toast'

import { parseIdArray } from '../lib/credential-refs'
import {
  downstreamKeysQueryKeys,
  type CreateDownstreamKeyResponse,
  type DownstreamApiKeyItem,
} from '../types'
import { KeyScopeFields, KeyAccessPolicyField } from './key-access-fields'
import { KeyIdentityFields, KeyLimitFields } from './key-details-fields'
import { createKeyPayload, updateKeyPayload } from './key-form-payload'
import {
  CREATE_FORM_ID,
  blankKeyFormValues,
  createKeySchema,
  editKeySchema,
  keyFormValuesFromItem,
  resolveKeyScopeControl,
  resolveApiErrorMessage,
  type CreateKeyFormValues,
} from './key-form-shared'
import { KeyPolicyPresetMenu } from './key-policy-preset-menu'
import { KeyRestrictionSummary } from './key-restriction-summary'

// `onCreated` (create mode only) receives the created key's export-dialog
// target straight from the create response so KeysSection can auto-open the
// Connect surface — the operator no longer has to hunt the row down.
export function KeySheetForm({
  editingKey,
  onDone,
  onCreated,
  onDirtyChange,
  candidateModels = [],
  initialLocateField = null,
}: {
  editingKey: DownstreamApiKeyItem | null
  onDone: () => void
  onCreated?: (target: CredentialExportTarget) => void
  /** Reports RHF dirty state so the hosting Sheet can guard dirty closes. */
  onDirtyChange?: (dirty: boolean) => void
  candidateModels?: string[]
  /** #1430: when the sheet was opened from a list-row restriction badge, the
   *  `data-key-scope` anchor of that dimension — scrolled into view once on
   *  mount; focus is owned by the Sheet's initialFocus resolver. */
  initialLocateField?: string | null
}) {
  const { t } = useTranslation()
  const handleReauth = useReauthReplay()
  const queryClient = useQueryClient()
  const isEdit = editingKey !== null

  const form = useForm<CreateKeyFormValues>({
    resolver: zodResolver(
      editingKey ? editKeySchema : createKeySchema
    ) as never,
    defaultValues: editingKey
      ? keyFormValuesFromItem(editingKey)
      : blankKeyFormValues(),
  })

  // Keep the parent's dirty flag in sync; clearing it on unmount guarantees
  // a reopened sheet never inherits a stale "unsaved" state.
  const isFormDirty = form.formState.isDirty
  useEffect(() => {
    onDirtyChange?.(isFormDirty)
    return () => onDirtyChange?.(false)
  }, [isFormDirty, onDirtyChange])

  // #1430 badge click-through: scroll the originating dimension's section
  // into view once the form has painted. Focus itself is owned by the
  // Sheet's initialFocus resolver (the dialog focus trap would steal a
  // directly-focused control). Runs once per mount — the parent remounts the
  // form per key via the `key` prop.
  const initialLocate = initialLocateField
  useEffect(() => {
    if (!initialLocate) return
    const frame = requestAnimationFrame(() => {
      resolveKeyScopeControl(initialLocate)?.scrollIntoView?.({
        block: 'center',
      })
    })
    return () => cancelAnimationFrame(frame)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const routeGrantCount = parseIdArray(editingKey?.allowedRouteIds).length

  const submitMutation = useMutation({
    mutationFn: async (input: {
      values: CreateKeyFormValues
      confirmToken?: string
    }) => {
      const values = input.values
      if (!editingKey) {
        const payload = createKeyPayload(values)
        return input.confirmToken
          ? api.createDownstreamApiKey(payload, input.confirmToken)
          : api.createDownstreamApiKey(payload)
      }
      const payload = updateKeyPayload(values)
      return input.confirmToken
        ? api.updateDownstreamApiKey(editingKey.id, payload, input.confirmToken)
        : api.updateDownstreamApiKey(editingKey.id, payload)
    },
    onSuccess: (result, input) => {
      const values = input.values
      void queryClient.invalidateQueries({
        queryKey: downstreamKeysQueryKeys.all,
      })
      if (isEdit) {
        toast.success(t('settings.downstream.keys.toast.updated'))
        onDone()
        return
      }
      // The dialog fetches the credential after confirmation.
      const createdItem = (result as CreateDownstreamKeyResponse | undefined)
        ?.item
      if (createdItem && createdItem.id > 0) {
        const target: CredentialExportTarget = {
          id: createdItem.id,
          name: createdItem.name || values.name,
          keyMasked: createdItem.keyMasked,
        }
        if (onCreated) {
          onCreated(target)
        } else {
          toast.success(t('settings.downstream.keys.toast.created'))
        }
      } else {
        toast.success(t('settings.downstream.keys.toast.created'))
      }
      onDone()
    },
    onError: (error, input) => {
      // A new/rotated downstream key is a long-lived credential: the backend
      // may demand the master token. Replay the same form values once; the
      // sheet keeps its input because the form is never reset here.
      if (
        handleReauth(
          error,
          t('reauth.operations.downstreamKey'),
          (confirmToken) =>
            submitMutation.mutateAsync({
              values: input.values,
              confirmToken,
            })
        )
      ) {
        return
      }
      // skipErrorHandler owns this call's feedback: surface the backend's
      // own message when present (400s carry the offending credentialRefs
      // entry index / unknown id), fall back to the generic toast.
      const serverMessage = resolveApiErrorMessage(error)
      toast.error(
        serverMessage ??
          (isEdit
            ? t('settings.downstream.keys.toast.updateFailed')
            : t('settings.downstream.keys.toast.createFailed'))
      )
    },
  })

  function onSubmit(values: CreateKeyFormValues) {
    submitMutation.mutate({ values })
  }

  function resolveSubmitLabel(): string {
    if (submitMutation.isPending) return t('settings.common.saving')
    if (isEdit) return t('settings.common.save')
    return t('settings.common.create')
  }

  return (
    <>
      <SheetHeader>
        <SheetTitle>
          {isEdit
            ? t('settings.downstream.keys.editTitle')
            : t('settings.downstream.keys.createTitle')}
        </SheetTitle>
        <SheetDescription>
          {isEdit
            ? t('settings.downstream.keys.editDescription')
            : t('settings.downstream.keys.createDescription')}
        </SheetDescription>
      </SheetHeader>
      <Form {...form}>
        <form
          id={CREATE_FORM_ID}
          onSubmit={form.handleSubmit(onSubmit)}
          className='min-h-0 flex-1 space-y-4 overflow-y-auto px-4'
        >
          <KeyRestrictionSummary routeGrantCount={routeGrantCount} />
          <KeyPolicyPresetMenu />
          <KeyIdentityFields isEdit={isEdit} />
          <KeyScopeFields
            candidateModels={candidateModels}
            routeGrantCount={routeGrantCount}
          />
          <KeyLimitFields isEdit={isEdit} />
          <KeyAccessPolicyField candidateModels={candidateModels} />
        </form>
      </Form>
      <SheetFooter>
        <Button
          type='submit'
          form={CREATE_FORM_ID}
          disabled={submitMutation.isPending}
        >
          {resolveSubmitLabel()}
        </Button>
      </SheetFooter>
    </>
  )
}
