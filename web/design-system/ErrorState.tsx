import type { ReactNode } from 'react';
import { EmptyState } from './EmptyState.js';
import { Button } from './Button.js';
import { cx } from './utils.js';

export type ErrorStateProps = {
  /** Defaults to an alert-triangle icon. */
  icon?: ReactNode;
  title: ReactNode;
  description?: ReactNode;
  /** When provided, a retry button is rendered. */
  onRetry?: () => void;
  /** Label for the retry button (caller-localized). */
  retryLabel?: string;
  className?: string;
};

/**
 * Standard error surface for data panels / pages. Mirrors New API
 * `ErrorState` (defaults to destructive icon + auto Retry) layered on the
 * shared `ds-empty` primitive. Token-only; callers pass localized strings.
 * `role="alert"` comes from `EmptyState` `tone="danger"`.
 */
export function ErrorState({
  icon,
  title,
  description,
  onRetry,
  retryLabel = '重试',
  className,
}: ErrorStateProps) {
  const action = onRetry ? (
    <Button variant="primary" size="md" onClick={onRetry}>
      {retryLabel}
    </Button>
  ) : undefined;
  return (
    <EmptyState
      tone="danger"
      icon={icon ?? <DefaultErrorIcon />}
      title={title}
      description={description}
      action={action}
      className={cx('ds-error-state', className)}
    />
  );
}

function DefaultErrorIcon() {
  return (
    <svg width="24" height="24" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M12 9v2m0 4h.01M5.07 19h13.86c1.54 0 2.5-1.67 1.73-3L13.73 4a2 2 0 00-3.46 0L3.34 16c-.77 1.33.19 3 1.73 3z" />
    </svg>
  );
}

export default ErrorState;
