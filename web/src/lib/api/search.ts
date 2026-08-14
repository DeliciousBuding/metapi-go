import { request } from './transport'

export const searchApi = {
  // Search
  search: (query: string) =>
    request('/api/search', {
      method: 'POST',
      body: JSON.stringify({ query, limit: 20 }),
    }),
}
