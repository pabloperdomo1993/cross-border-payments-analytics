import { useQuery } from '@tanstack/react-query'

import { fetchCorridors, fetchProviders, fetchTimeSeries } from '../api/analyticsApi'
import type { AnalyticsFilters } from '../types'

// Query keys include the filters object, so changing any filter
// naturally produces a new cache entry/refetch — no manual
// invalidation needed.

export function useCorridors(filters: AnalyticsFilters) {
  return useQuery({
    queryKey: ['analytics', 'corridors', filters],
    queryFn: ({ signal }) => fetchCorridors(filters, signal),
  })
}

export function useProviders(filters: AnalyticsFilters) {
  return useQuery({
    queryKey: ['analytics', 'providers', filters],
    queryFn: ({ signal }) => fetchProviders(filters, signal),
  })
}

export function useTimeSeries(filters: AnalyticsFilters) {
  return useQuery({
    queryKey: ['analytics', 'timeseries', filters],
    queryFn: ({ signal }) => fetchTimeSeries(filters, signal),
  })
}
