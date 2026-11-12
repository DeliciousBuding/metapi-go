import { Navigate } from '@tanstack/react-router'

// Preserve saved settings links while keeping one audit workspace.
export function AuditLogsSection() {
  return (
    <Navigate to='/audit-logs' search={{ category: 'operations' }} replace />
  )
}
