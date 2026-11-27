import { type ChangeEvent, useRef, useState } from 'react'

export function useImportFields() {
  // Fields flagged invalid by a failed validation pass. Cleared per-field
  // the moment the user edits that field, so an error never outlives the
  // input that caused it. The single source of truth for aria-invalid on
  // wizard inputs — no new error visuals, the design system already wires
  // aria-invalid styling into Input/Textarea.
  const [invalidFields, setInvalidFields] = useState<Set<string>>(
    () => new Set()
  )

  // Live registry of focusable field elements keyed by a stable field id
  // ("source", `${candidate.id}-platform`, `${candidate.id}-weight`). Ref
  // callbacks keep this current on mount/unmount so the focus target for
  // "first invalid field" is always the committed DOM node. Callbacks are
  // cached per key so React does not tear down and reattach on each render.
  const fieldRefs = useRef<Map<string, HTMLElement>>(new Map())
  const fieldRefCallbacks = useRef<
    Map<string, (element: HTMLElement | null) => void>
  >(new Map())
  function registerFieldRef(key: string) {
    let callback = fieldRefCallbacks.current.get(key)
    if (callback === undefined) {
      callback = (element) => {
        if (element) {
          fieldRefs.current.set(key, element)
        } else {
          fieldRefs.current.delete(key)
        }
      }
      fieldRefCallbacks.current.set(key, callback)
    }
    return callback
  }

  function markInvalidAndFocusFirst(keys: string[]) {
    setInvalidFields(new Set(keys))
    const firstKey = keys[0]
    if (firstKey) fieldRefs.current.get(firstKey)?.focus()
  }

  function bindField(key: string, onValueChange: (value: string) => void) {
    const invalid = invalidFields.has(key)
    return {
      ref: registerFieldRef(key),
      'aria-invalid': invalid || undefined,
      onChange: (
        event: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>
      ) => {
        onValueChange(event.target.value)
        if (invalid) {
          setInvalidFields((previous) => {
            if (!previous.has(key)) return previous
            const next = new Set(previous)
            next.delete(key)
            return next
          })
        }
      },
    }
  }

  return {
    bindField,
    markInvalidAndFocusFirst,
    clearInvalid: () => setInvalidFields(new Set()),
  }
}

export type BindImportField = ReturnType<typeof useImportFields>['bindField']
