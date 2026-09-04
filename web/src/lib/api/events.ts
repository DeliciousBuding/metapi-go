import { request } from './transport'

export const eventsApi = {
  // Events
  getEvents: (params?: string) =>
    request(`/api/events${params ? `?${params}` : ''}`),
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
  getTask: (id: string) => request(`/api/tasks/${encodeURIComponent(id)}`),
}
