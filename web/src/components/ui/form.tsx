// metapi-go/ui — Form: react-hook-form wiring for metapi's field layout
// (base-nova style, @base-ui/react). Based on shadcn/ui form (MIT); the
// form-scoped submit-error focus, the Base UI render delegation and the
// translated message bodies are metapi-go's own.
//
// Ownership map — one concern, one owner:
//   Form         a selector-safe scope id, published through context
//   FormField    the field name (an RHF Controller)
//   FormItem     the per-field id, and the scope stamp on the DOM
//   FormControl  the only slot that touches the rendered control
//   FormLabel    htmlFor, plus the id composite controls label themselves by
//
// Submit-error focus is scoped by that id rather than by DOM ancestry because a
// sheet form and a page form can be mounted at the same time: an unscoped
// `[aria-invalid="true"]` lookup would happily focus the field hidden behind the
// sheet.
import { useRender } from '@base-ui/react/use-render'
import * as React from 'react'
import {
  Controller,
  FormProvider,
  useFormContext,
  useFormState,
  type ControllerProps,
  type FieldPath,
  type FieldValues,
} from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'

const INVALID_SELECTOR = '[aria-invalid="true"]'
const MESSAGE_SELECTOR = '[data-slot="form-message"]'
const FOCUSABLE_SELECTOR =
  'a[href], button, input, select, textarea, [tabindex]:not([tabindex="-1"])'

const FormScopeContext = React.createContext<string | null>(null)

/**
 * `React.useId()` is unique but not selector-safe (it emits `:`), so it is
 * normalised once here instead of being escaped at every lookup.
 */
function toScopeId(reactId: string): string {
  return `form-${reactId.replaceAll(/[^a-zA-Z0-9_-]/g, '_')}`
}

function hasFieldErrors(errors: unknown): boolean {
  return (
    typeof errors === 'object' &&
    errors !== null &&
    Object.keys(errors).length > 0
  )
}

/**
 * Scrolls the first errored field of `scopeId` into view and focuses it.
 *
 * "First" is document order, which is what a keyboard or screen-reader user
 * expects after a rejected submit. When the invalid node is not itself focusable
 * — a composite `role="group"` control, or an error message with no marked
 * control — focus lands on the first focusable element of the same item, so the
 * announcement is never left dangling on an unfocusable node.
 */
function focusFirstInvalidField(scopeId: string): void {
  const scopedItems = `[data-form-id="${scopeId}"][data-slot="form-item"]`

  for (const item of document.querySelectorAll<HTMLElement>(scopedItems)) {
    const invalid =
      item.querySelector<HTMLElement>(INVALID_SELECTOR) ??
      item.querySelector<HTMLElement>(MESSAGE_SELECTOR)
    if (!invalid) continue

    item.scrollIntoView({ block: 'center', behavior: 'smooth' })
    const focusTarget = invalid.matches(FOCUSABLE_SELECTOR)
      ? invalid
      : item.querySelector<HTMLElement>(FOCUSABLE_SELECTOR)
    focusTarget?.focus({ preventScroll: true })
    return
  }
}

/**
 * Renderless: moves focus to the first invalid field after a rejected submit.
 *
 * react-hook-form reports failure as state rather than as an event, so there is
 * no callback to hang this on. The effect keys off `submitCount` and records the
 * submit it already handled — without that, every keystroke re-rendering the
 * form while errors persist would drag the user back to the field. The lookup
 * waits a frame so it sees the error markup this submit just mounted.
 */
function SubmitErrorFocus() {
  const scopeId = React.useContext(FormScopeContext)
  const { control } = useFormContext()
  const { errors, submitCount } = useFormState({ control })
  const handledSubmit = React.useRef(0)

  React.useEffect(() => {
    if (!scopeId || submitCount === 0 || !hasFieldErrors(errors)) return
    if (handledSubmit.current === submitCount) return

    handledSubmit.current = submitCount

    const frame = window.requestAnimationFrame(() =>
      focusFirstInvalidField(scopeId)
    )
    return () => window.cancelAnimationFrame(frame)
  }, [errors, scopeId, submitCount])

  return null
}

function Form<TFieldValues extends FieldValues = FieldValues>({
  children,
  ...props
}: React.ComponentProps<typeof FormProvider<TFieldValues>>) {
  const scopeId = toScopeId(React.useId())

  return (
    <FormScopeContext.Provider value={scopeId}>
      <FormProvider {...props}>
        <SubmitErrorFocus />
        {children}
      </FormProvider>
    </FormScopeContext.Provider>
  )
}

type FormFieldContextValue<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>,
> = {
  name: TName
}

const FormFieldContext = React.createContext<FormFieldContextValue>(
  {} as FormFieldContextValue
)

const FormField = <
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>,
>({
  ...props
}: ControllerProps<TFieldValues, TName>) => {
  return (
    <FormFieldContext.Provider value={{ name: props.name }}>
      <Controller {...props} />
    </FormFieldContext.Provider>
  )
}

const FormItemContext = React.createContext<string>('')

/**
 * Field state plus the ids this field's slots share. Usable outside `FormField`
 * for fields that are not RHF-controlled (`test-form.tsx` wires a plain Select
 * this way): `name` is then empty and no error is reported, while the label /
 * description / message ids stay consistent.
 */
function useFormField() {
  const { name } = React.useContext(FormFieldContext)
  const id = React.useContext(FormItemContext)
  const { getFieldState } = useFormContext()
  const formState = useFormState({ name })

  return {
    id,
    name,
    formItemId: `${id}-form-item`,
    formDescriptionId: `${id}-form-item-description`,
    formMessageId: `${id}-form-item-message`,
    ...getFieldState(name, formState),
  }
}

function FormItem({ className, ...props }: React.ComponentProps<'div'>) {
  const itemId = React.useId()
  const scopeId = React.useContext(FormScopeContext)

  return (
    <FormItemContext.Provider value={itemId}>
      <div
        data-slot='form-item'
        data-form-id={scopeId ?? undefined}
        className={cn('grid gap-2', className)}
        {...props}
      />
    </FormItemContext.Provider>
  )
}

/**
 * The id `FormLabel` renders, derived from the same `formItemId` that
 * `FormControl` injects onto its single child. A native control is named by the
 * `<label htmlFor>`; a *composite* control (a group of inputs with no single
 * labelable element) instead forwards that injected id onto its `role="group"`
 * root and points `aria-labelledby` here, so the group gets an accessible name
 * without reaching back into form context. One owner for the `-label` suffix
 * convention shared by FormLabel and the composite fields (#1300).
 */
function formLabelIdFor(formItemId: string): string {
  return `${formItemId}-label`
}

function FormLabel({
  className,
  ...props
}: React.ComponentProps<typeof Label>) {
  const { error, formItemId } = useFormField()

  return (
    <Label
      id={formLabelIdFor(formItemId)}
      data-slot='form-label'
      data-error={!!error}
      className={cn('data-[error=true]:text-destructive', className)}
      htmlFor={formItemId}
      {...props}
    />
  )
}

function FormControl({
  children,
  ...props
}: { children: React.ReactElement } & Record<string, unknown>) {
  const { error, formItemId, formDescriptionId, formMessageId } = useFormField()

  return useRender({
    render: children,
    props: {
      'data-slot': 'form-control',
      id: formItemId,
      'aria-invalid': !!error,
      // The message joins aria-describedby only once it exists, so an errored
      // field is announced with its reason and a clean one is not left pointing
      // at an id that renders nothing.
      'aria-describedby': error
        ? `${formDescriptionId} ${formMessageId}`
        : formDescriptionId,
      ...props,
    },
  })
}

function FormDescription({ className, ...props }: React.ComponentProps<'p'>) {
  const { formDescriptionId } = useFormField()

  return (
    <p
      data-slot='form-description'
      id={formDescriptionId}
      className={cn('text-muted-foreground text-sm', className)}
      {...props}
    />
  )
}

function FormMessage({
  className,
  children,
  ...props
}: React.ComponentProps<'p'>) {
  const { error, formMessageId } = useFormField()
  const { t } = useTranslation()

  // The field error is the message body; `children` is the fallback for a note a
  // caller renders itself. Error text arrives as an i18n key (from zod or from
  // the API), so it goes through t() — an unknown key comes back verbatim.
  const body = error ? String(error.message ?? '') : children
  if (!body) return null

  return (
    <p
      data-slot='form-message'
      id={formMessageId}
      // Assertive live region: errors mount after a failed submit, and the
      // role makes screen readers announce them without waiting for focus to
      // reach the (aria-describedby-linked) control.
      role='alert'
      className={cn('text-destructive text-sm', className)}
      {...props}
    >
      {typeof body === 'string' ? t(body) : body}
    </p>
  )
}

export {
  Form,
  FormItem,
  FormLabel,
  FormControl,
  FormDescription,
  FormMessage,
  FormField,
  formLabelIdFor,
}
