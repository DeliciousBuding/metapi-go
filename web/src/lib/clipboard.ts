// metapi-go/lib — shared clipboard write helper.
//
// navigator.clipboard is unavailable outside secure contexts (or when the
// browser denies the permission), so every copy affordance must handle the
// failure path. This helper centralizes the guard: callers receive a boolean
// and own their own feedback (copied flash, success/failure toasts).

export async function copyText(text: string): Promise<boolean> {
  if (typeof navigator === 'undefined' || !navigator.clipboard?.writeText) {
    return false
  }
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    // Clipboard write rejected (permissions / non-secure context).
    return false
  }
}
