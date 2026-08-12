// metapi-go/features/model-tester — Zod form schema.
//
// Validates the test form: model (required), system prompt (optional),
// user prompt (required), target protocol format, and sampling parameters
// (temperature / top_p / max_tokens / stream). Numeric fields use plain
// `z.number()` (no `z.coerce`/`.default()`) so the zod resolver's input and
// output types match and RHF typing stays clean; the form binds them with an
// explicit `onChange` that converts via `valueAsNumber`. Error messages are
// i18next keys (resolved by `<FormMessage>`).

import { z } from 'zod'

import type { TestTargetFormat } from '../types'

const TARGET_FORMATS = [
  'openai',
  'claude',
  'responses',
  'gemini',
] as const satisfies readonly TestTargetFormat[]

export const testerSchema = z.object({
  model: z.string().trim().min(1, 'modelTester.form.errors.modelRequired'),
  systemPrompt: z.string().max(4000, 'modelTester.form.errors.systemTooLong'),
  prompt: z
    .string()
    .trim()
    .min(1, 'modelTester.form.errors.promptRequired')
    .max(16000, 'modelTester.form.errors.promptTooLong'),
  targetFormat: z.enum(TARGET_FORMATS),
  temperature: z
    .number()
    .min(0, 'modelTester.form.errors.temperatureMin')
    .max(2, 'modelTester.form.errors.temperatureMax'),
  topP: z
    .number()
    .min(0, 'modelTester.form.errors.topPMin')
    .max(1, 'modelTester.form.errors.topPMax'),
  maxTokens: z
    .number()
    .int('modelTester.form.errors.maxTokensInteger')
    .min(0, 'modelTester.form.errors.maxTokensMin')
    .max(128000, 'modelTester.form.errors.maxTokensMax'),
  stream: z.boolean(),
})

export type TesterFormValues = z.infer<typeof testerSchema>

export const TESTER_FORM_DEFAULT_VALUES: TesterFormValues = {
  model: '',
  systemPrompt: '',
  prompt: '',
  targetFormat: 'openai',
  temperature: 0.7,
  topP: 1,
  maxTokens: 1024,
  stream: false,
}
