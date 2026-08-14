import { request } from './transport'

export const eventsApi = {
  // Events
  getEvents: (params?: string) =>
    request(`/api/events${params ? `?${params}` : ''}`),
  getEventCount: () => request('/api/events/count'),
  markEventRead: (id: number) =>
    request(`/api/events/${id}/read`, { method: 'POST' }),
  markAllEventsRead: () => request('/api/events/read-all', { method: 'POST' }),
  clearEvents: () => request('/api/events', { method: 'DELETE' }),
  getTasks: (limit = 50) =>
    request(
      `/api/tasks?limit=${Math.max(1, Math.min(200, Math.trunc(limit)))}`
    ),
  getTask: (id: string) => request(`/api/tasks/${encodeURIComponent(id)}`),
}
