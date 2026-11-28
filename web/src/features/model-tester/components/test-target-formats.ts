import type { TesterFormValues } from '../lib/tester-schema'

export const TARGET_FORMAT_OPTIONS: Array<{
  value: TesterFormValues['targetFormat']
  labelKey: string
}> = [
  { value: 'openai', labelKey: 'modelTester.form.targetFormat.openai' },
  { value: 'claude', labelKey: 'modelTester.form.targetFormat.claude' },
  { value: 'responses', labelKey: 'modelTester.form.targetFormat.responses' },
  { value: 'gemini', labelKey: 'modelTester.form.targetFormat.gemini' },
]
