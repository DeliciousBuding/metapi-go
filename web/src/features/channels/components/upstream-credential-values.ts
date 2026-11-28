import { z } from 'zod'

import type { ImportedCredentialUpdate } from '@/lib/api/imported-upstreams'

export const credentialSchema = z
  .object({
    name: z.string().trim().min(1, 'channels.upstream.required'),
    kind: z.enum(['api_key', 'oauth', 'none', 'gcp_service_account']),
    gcpServiceAccount: z.string(),
    enabled: z.boolean(),
    replace: z.boolean(),
    accessToken: z.string(),
    refreshToken: z.string(),
    clientId: z.string(),
    idToken: z.string(),
    accountId: z.string(),
    projectId: z.string(),
    expiresAt: z
      .string()
      .refine(
        (value) =>
          !value ||
          (Number.isFinite(new Date(value).getTime()) &&
            new Date(value).getTime() > 0),
        'channels.upstream.invalidExpiry'
      ),
  })
  .refine(
    (value) =>
      value.kind === 'none' ||
      value.kind === 'gcp_service_account' ||
      !value.replace ||
      !!value.accessToken.trim() ||
      (value.kind === 'oauth' &&
        !!value.refreshToken.trim() &&
        !!value.projectId.trim()),
    {
      path: ['accessToken'],
      message: 'channels.upstream.required',
    }
  )
  .refine(
    (value) =>
      value.kind !== 'gcp_service_account' ||
      !value.replace ||
      !!value.gcpServiceAccount.trim(),
    {
      path: ['gcpServiceAccount'],
      message: 'channels.upstream.required',
    }
  )
export type CredentialValues = z.infer<typeof credentialSchema>

// Replacement material is write-only, including when an existing credential is edited.
export const emptyCredentialInputs = {
  accessToken: '',
  refreshToken: '',
  clientId: '',
  idToken: '',
  accountId: '',
  projectId: '',
  expiresAt: '',
  gcpServiceAccount: '',
}

export function credentialToPatch(
  values: CredentialValues,
  dirtyFields?: { name?: boolean; enabled?: boolean }
): ImportedCredentialUpdate {
  const patch: ImportedCredentialUpdate = {}
  if (!dirtyFields || dirtyFields.name) {
    patch.name = values.name
  }
  if (!dirtyFields || dirtyFields.enabled) {
    patch.enabled = values.enabled
  }
  if (values.replace) {
    if (values.kind === 'api_key') patch.apiKey = values.accessToken
    else if (values.kind === 'none') patch.kind = 'none'
    else if (values.kind === 'gcp_service_account') {
      patch.kind = 'gcp_service_account'
      patch.gcpServiceAccount = values.gcpServiceAccount.trim()
    } else {
      patch.oauth = {
        accessToken: values.accessToken,
        refreshToken: values.refreshToken || undefined,
        clientId: values.clientId || undefined,
        idToken: values.idToken || undefined,
        accountId: values.accountId || undefined,
        projectId: values.projectId || undefined,
        expiresAt: values.expiresAt
          ? new Date(values.expiresAt).getTime()
          : undefined,
      }
    }
  }
  return patch
}
