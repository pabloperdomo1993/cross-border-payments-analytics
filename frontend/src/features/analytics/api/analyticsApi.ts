import { analyticsApiClient, type RequestOptions } from '../../../lib/apiClient'
import type {
  AnalyticsFilters,
  CorridorVolume,
  ProviderStats,
  TimeSeriesPoint,
} from '../types'

/** Maps the app's camelCase filter model onto analytics-service's real
 * (snake_case) query parameter names — the one place that translation
 * happens, so every hook/component works with the same typed filters. */
function toQuery(filters: AnalyticsFilters): RequestOptions['query'] {
  return {
    from: filters.from,
    to: filters.to,
    source_country: filters.sourceCountry,
    destination_country: filters.destinationCountry,
    source_currency: filters.sourceCurrency,
    destination_currency: filters.destinationCurrency,
    provider: filters.provider,
    status: filters.status,
  }
}

interface CorridorsResponse {
  corridors: CorridorVolume[]
}

interface ProvidersResponse {
  providers: ProviderStats[]
}

interface TimeSeriesResponse {
  timeseries: TimeSeriesPoint[]
  interval: string
}

export function fetchCorridors(filters: AnalyticsFilters, signal?: AbortSignal): Promise<CorridorVolume[]> {
  return analyticsApiClient
    .get<CorridorsResponse>('/api/v1/analytics/corridors', { query: toQuery(filters), signal })
    .then((res) => res.corridors)
}

export function fetchProviders(filters: AnalyticsFilters, signal?: AbortSignal): Promise<ProviderStats[]> {
  return analyticsApiClient
    .get<ProvidersResponse>('/api/v1/analytics/providers', { query: toQuery(filters), signal })
    .then((res) => res.providers)
}

export function fetchTimeSeries(filters: AnalyticsFilters, signal?: AbortSignal): Promise<TimeSeriesPoint[]> {
  return analyticsApiClient
    .get<TimeSeriesResponse>('/api/v1/analytics/timeseries', {
      query: { ...toQuery(filters), interval: 'day' },
      signal,
    })
    .then((res) => res.timeseries)
}
