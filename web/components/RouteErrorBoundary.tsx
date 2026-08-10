import React from 'react';
import { tr } from '../i18n.js';
import { useLocation } from 'react-router-dom';

interface ErrorBoundaryProps {
  children: React.ReactNode;
}

interface ErrorBoundaryState {
  error: Error | null;
}

/**
 * Prevents a single lazy-loaded route render error from blank-screening the
 * whole SPA. Mirrors the New API per-route `errorComponent` pattern (see
 * `analysis/uiux-newapi-borrow-2026-07-30.md` §B1) without adopting TanStack.
 *
 * The `useRouteErrorBoundary` wrapper keys on `location.pathname` so navigating
 * away from the broken route resets the boundary automatically.
 */
class ErrorBoundaryInner extends React.Component<ErrorBoundaryProps & { resetKey: string }, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: React.ErrorInfo): void {
    // Best-effort log; the SPA has no telemetry sink.
    console.error('Route render error:', error, info);
  }

  componentDidUpdate(prevProps: Readonly<ErrorBoundaryProps & { resetKey: string }>): void {
    // Clear the error when the route changes so a retry is possible without a reload.
    if (this.state.error && prevProps.resetKey !== this.props.resetKey) {
      this.setState({ error: null });
    }
  }

  render(): React.ReactNode {
    if (this.state.error) {
      return <RouteErrorView error={this.state.error} onRetry={() => this.setState({ error: null })} />;
    }
    return this.props.children;
  }
}

function RouteErrorView({ error, onRetry }: { error: Error; onRetry: () => void }) {
  return (
    <div className="route-error-boundary" role="alert" aria-live="assertive">
      <svg width="40" height="40" fill="none" viewBox="0 0 24 24" stroke="var(--color-text-muted)" aria-hidden="true">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.8} d="M12 9v2m0 4h.01M5.07 19h13.86c1.54 0 2.5-1.67 1.73-3L13.73 4a2 2 0 00-3.46 0L3.34 16c-.77 1.33.19 3 1.73 3z" />
      </svg>
      <div className="route-error-title">{tr('页面渲染出错')}</div>
      <div className="route-error-message">{error.message || String(error)}</div>
      <button type="button" className="btn btn-primary" onClick={onRetry}>{tr('重试')}</button>
    </div>
  );
}

export function RouteErrorBoundary({ children }: ErrorBoundaryProps) {
  const location = useLocation();
  return (
    <ErrorBoundaryInner resetKey={location.pathname}>{children}</ErrorBoundaryInner>
  );
}

export default RouteErrorBoundary;
