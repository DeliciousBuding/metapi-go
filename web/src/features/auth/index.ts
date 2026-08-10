// metapi-go/features/auth — barrel re-exports.

export { SignInPage } from './components/sign-in-page'
export { LoginForm } from './components/login-form'
export { useLogin, validateAdminToken, resolveLoginErrorMessageKey } from './api'
export { loginFormSchema, type LoginFormValues } from './lib/login-schema'
export { sanitizeAuthRedirect } from './lib/auth-redirect'

export type {
  AuthBundle,
  AuthUser,
  LoginSession,
  LoginPayload,
  LoginError,
  AuthFormProps,
} from './types'
