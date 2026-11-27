import { serializeCredentialRefs } from '../lib/credential-refs'
import {
  localDatetimeInputToIso,
  type CreateKeyFormValues,
} from './key-form-shared'

// Create and update both send credential refs as arrays and expiry as RFC3339 UTC.
function keyPolicyAndExpiry(values: CreateKeyFormValues) {
  return {
    expiresAt: localDatetimeInputToIso(values.expiresAt ?? ''),
    accessPolicy: values.accessPolicy,
    allowedCredentialRefs: serializeCredentialRefs(
      values.allowedCredentialRefs
    ),
    excludedCredentialRefs: serializeCredentialRefs(
      values.excludedCredentialRefs
    ),
  }
}

export function createKeyPayload(values: CreateKeyFormValues) {
  return { ...values, ...keyPolicyAndExpiry(values) }
}

export function updateKeyPayload(values: CreateKeyFormValues) {
  return {
    name: values.name,
    groupName: values.groupName,
    maxRequests: values.maxRequests,
    maxCost: values.maxCost,
    enabled: values.enabled,
    supportedModels: values.supportedModels,
    allowedSiteIds: values.allowedSiteIds,
    ...keyPolicyAndExpiry(values),
  }
}
