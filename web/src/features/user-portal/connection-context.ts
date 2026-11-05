import type { PersonalModel } from './api'

export type PersonalProtocol = PersonalModel['protocols'][number]
export type ModelConnectionContext = {
  model?: string
  protocol?: PersonalProtocol
  invalid?: boolean
}

const allowedProtocols: readonly PersonalProtocol[] = [
  'chat/completions',
  'responses',
  'messages',
  'embeddings',
]

// Navigation carries public model/protocol choices only. Reject ambiguous or
// malformed values rather than truncating them into a different model name.
export function readModelContext(search: string): ModelConnectionContext {
  const params = new URLSearchParams(search)
  const models = params.getAll('model')
  const protocols = params.getAll('protocol')
  if (models.length === 0 && protocols.length === 0) return {}
  const model = models[0]?.trim()
  const protocol = protocols[0]
  if (
    models.length !== 1 ||
    !model ||
    [...model].length > 200 ||
    [...model].some((value) => {
      const code = value.codePointAt(0)
      return code !== undefined && (code < 32 || code === 127)
    }) ||
    protocols.length > 1 ||
    (protocol !== undefined &&
      !allowedProtocols.includes(protocol as PersonalProtocol))
  )
    return { invalid: true }
  return { model, protocol: protocol as PersonalProtocol | undefined }
}

export function modelContextHref(
  search: string,
  context: ModelConnectionContext
): string {
  const params = new URLSearchParams(search)
  if (context.model) params.set('model', context.model)
  else params.delete('model')
  if (context.protocol) params.set('protocol', context.protocol)
  else params.delete('protocol')
  return '/user?' + params.toString()
}
