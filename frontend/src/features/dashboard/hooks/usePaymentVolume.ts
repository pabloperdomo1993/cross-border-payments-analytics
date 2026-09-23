import { useTimeSeries } from '../../analytics/hooks/useAnalyticsQueries'
import type { AnalyticsFilters } from '../../analytics/types'

/** Thin re-export under a dashboard-facing name — the chart doesn't
 * need to know this data is shared with the analytics feature. */
export function usePaymentVolume(filters: AnalyticsFilters) {
  return useTimeSeries(filters)
}
