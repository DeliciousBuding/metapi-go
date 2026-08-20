import { request } from './transport'

export const eventsApi = {
  // Events
  getEvents: (params?: string) =>
    request(`/api/events${params ? `?${params}` : ''}`),
  getEventCount: () => request('/api/events/count'),
  markEventRead: (id: number) =>
    request(`/api/events/${id}/read`, {
      method: 'POST',
      // The program-logs section surfaces its own markReadFailed toast.
      skipErrorHandler: true,
    }),
  markAllEventsRead: () =>
    request('/api/events/read-all', {
      method: 'POST',
      // The program-logs section surfaces its own markAllFailed toast.
      skipErrorHandler: true,
    }),
  clearEvents: () =>
    request('/api/events', {
      method: 'DELETE',
      // The program-logs section surfaces its own clearFailed toast.
      skipErrorHandler: true,
    }),
  getTasks: (limit = 50) =>
    request(
      `/api/tasks?limit=${Math.max(1, Math.min(200, Math.trunc(limit)))}`
    ),
  getTask: (id: string) => request(`/api/tasks/${encodeURIComponent(id)}`),
}
