import { cx } from './utils.js';

export type LoadingStateProps = {
  /** Inline = small spinner + label beside other content; block (default) = skeleton lines. */
  inline?: boolean;
  /** Label / aria-label for the region. */
  label?: string;
  /** Number of skeleton lines for the block variant. */
  lines?: number;
  className?: string;
};

/**
 * Standard loading surface. Block variant renders N skeleton bars reusing the
 * existing `.skeleton` token; inline variant renders a small spinner + label.
 * Both expose `role="status" aria-live="polite"` for screen readers. Mirrors
 * New API `LoadingState` (inline variant) + `ContentSkeleton` (block).
 */
export function LoadingState({ inline = false, label, lines = 3, className }: LoadingStateProps) {
  if (inline) {
    return (
      <span className={cx('ds-loading-inline', className)} role="status" aria-live="polite">
        <span className="spinner spinner-sm" aria-hidden="true" />
        {label != null && <span className="ds-loading-inline__label">{label}</span>}
      </span>
    );
  }
  const count = Math.max(1, Math.min(lines, 12));
  return (
    <div
      className={cx('ds-loading', className)}
      role="status"
      aria-live="polite"
      aria-label={label ?? '加载中'}
    >
      {Array.from({ length: count }).map((_, i) => (
        <div
          key={i}
          className="skeleton ds-loading__line"
          style={{ width: i === count - 1 ? '60%' : '100%' }}
        />
      ))}
    </div>
  );
}

export default LoadingState;
