// metapi-go/features/model-tester — barrel re-exports.
//
// Page component is the primary surface; the rest is exported for the
// future `/model-tester` route file and for the `useTestModel` hook reuse
// (e.g. a future quick-test button on the models detail sheet).

export { ModelTesterPage } from './components/model-tester-page'
export { TestForm } from './components/test-form'
export { TestResponseViewer } from './components/test-response-viewer'

export { useTestModel } from './api'
export {
  testerSchema,
  TESTER_FORM_DEFAULT_VALUES,
  type TesterFormValues,
} from './lib/tester-schema'

export {
  type TestTargetFormat,
  type TestFormValues,
  type TestModelVariables,
  type TestStreamDelta,
  type TestResponse,
} from './types'
