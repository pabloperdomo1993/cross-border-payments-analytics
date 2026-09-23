// Wire types match analytics-service's real JSON responses exactly
// (snake_case, decimal-string volumes) — see
// backend/analytics-service/internal/domain/event.go and
// internal/handlers/http/analytics_handler.go.

export interface CorridorVolume {
  source_country: string
  destination_country: string
  transaction_count: number
  total_volume: string
}

export interface CurrencyVolume {
  currency: string
  transaction_count: number
  total_volume: string
}

export interface CountryVolume {
  country: string
  transaction_count: number
  total_volume: string
}

export interface ProviderStats {
  provider: string
  total_transactions: number
  completed_transactions: number
  failed_transactions: number
  failure_rate: number
}

export interface TimeSeriesPoint {
  period_start: string
  transaction_count: number
  total_volume: string
}

/** Shared filter shape reused by every analytics query and the
 * (mocked) payments list — matches analytics-service's real query
 * parameters, so a filter change maps directly onto real API calls. */
export interface AnalyticsFilters {
  from?: string
  to?: string
  sourceCountry?: string
  destinationCountry?: string
  sourceCurrency?: string
  destinationCurrency?: string
  provider?: string
  status?: 'pending' | 'completed' | 'failed'
}

export const EMPTY_FILTERS: AnalyticsFilters = {}
