// metapi-go/lib — centralized toast helpers with per-variant duration tiers.
//
// Tiers (design SSOT PR1 D8):
//   success 3s · default (info/warning/message) 4s · error 5s
// Loading stays passthrough (loading toasts should persist until replaced).
// Callers may still override duration explicitly (their data is spread last).

import { toast as baseToast, type ExternalToast } from 'sonner'

type ToastInput = string | React.ReactNode

const SUCCESS_MS = 3000
const DEFAULT_MS = 4000
const ERROR_MS = 5000

function withDuration(
  fn: (message: ToastInput, data?: ExternalToast) => string | number,
  ms: number
) {
  return (message: ToastInput, data?: ExternalToast) =>
    fn(message, { duration: ms, ...data })
}

export const toast = {
  success: withDuration(baseToast.success, SUCCESS_MS),
  error: withDuration(baseToast.error, ERROR_MS),
  info: withDuration(baseToast.info, DEFAULT_MS),
  warning: withDuration(baseToast.warning, DEFAULT_MS),
  message: withDuration(baseToast.message, DEFAULT_MS),
  loading: baseToast.loading,
  custom: baseToast.custom,
  promise: baseToast.promise,
  dismiss: baseToast.dismiss,
}