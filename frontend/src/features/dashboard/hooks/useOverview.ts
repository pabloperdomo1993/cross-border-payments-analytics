import { useCorridors, useProviders } from '../../analytics/hooks/useAnalyticsQueries'
import type { AnalyticsFilters } from '../../analytics/types'
import { computeOverview } from '../api/overviewApi'
import type { Overview } from '../types'

export interface UseOverviewResult {
  data: Overview | undefined
  isLoading: boolean
  isError: boolean
  error: Error | null
  refetch: () => void
}

/**
 * Composes two real, independent queries (corridors + providers) into
 * one KPI object. Reuses `useCorridors`/`useProviders` rather than
 * fetching separately, so a corridor chart on the same page shares the
 * same cache entry instead of doubling the network calls.
 */
export function useOverview(filters: AnalyticsFilters): UseOverviewResult {
  const corridors = useCorridors(filters)
  const providers = useProviders(filters)

  const isLoading = corridors.isLoading || providers.isLoading
  const isError = corridors.isError || providers.isError

  return {
    data: corridors.data && providers.data ? computeOverview(corridors.data, providers.data) : undefined,
    isLoading,
    isError,
    error: (corridors.error ?? providers.error) as Error | null,
    refetch: () => {
      void corridors.refetch()
      void providers.refetch()
    },
  }
}
