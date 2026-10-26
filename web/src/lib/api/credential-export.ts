import { z } from 'zod'

const profileIdSchema = z.enum([
  'openai',
  'cherry',
  'generic',
  'claude-code',
  'codex',
  'openwebui',
])

const profileSchema = z
  .object({
    id: profileIdSchema,
    label: z.string().min(1),
    description: z.string().optional(),
    contentType: z.string().min(1),
    content: z.union([z.string(), z.record(z.string(), z.unknown())]),
    protocol: z.enum(['chat', 'responses', 'messages']).optional(),
    setup: z
      .object({ contentType: z.string().min(1), content: z.string() })
      .optional(),
  })
  .refine((profile) => {
    if (profile.id !== 'generic' && profile.id !== 'cherry') return true
    return (
      typeof profile.content === 'object' &&
      typeof profile.content.apiKey === 'string' &&
      profile.content.apiKey.length > 0
    )
  })

const exportSchema = z.object({
  success: z.literal(true),
  formatVersion: z.string().min(1),
  keyId: z.number().int().positive(),
  keyName: z.string(),
  baseUrl: z.url(),
  // Older exports omit model/protocol/setup; present fields must still be valid.
  model: z.string().optional(),
  profiles: z.array(profileSchema).min(1),
})

export type CredentialExportProfile = z.infer<typeof profileSchema>
export type CredentialExportResponse = z.infer<typeof exportSchema>
export type CredentialExportProfileId = z.infer<typeof profileIdSchema>

export function parseCredentialExport(
  value: unknown
): CredentialExportResponse {
  const result = exportSchema.safeParse(value)
  // Do not attach the rejected payload to errors: it can contain credentials.
  if (!result.success) throw new Error('Invalid downstream key export response')
  return result.data
}
