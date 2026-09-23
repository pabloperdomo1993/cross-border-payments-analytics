import type { AnalyticsFilters } from '../analytics/types'

/**
 * Mirrors payments-service's REAL single-transaction wire shape
 * exactly (see backend/payments-service/internal/handlers/http/
 * transaction_handler.go's transactionResponse) — snake_case,
 * decimal-string amounts. payments-service has no list endpoint today
 * (see PaymentsPage / mockPaymentsApi for why this is currently
 * served from a deterministic mock), but if one is added it would
 * almost certainly return rows shaped exactly like this, since it's
 * the service's own established convention.
 */
export interface Payment {
  id: string
  source_country: string
  destination_country: string
  source_currency: string
  destination_currency: string
  provider: string
  source_amount: string
  status: 'pending' | 'completed' | 'failed'
  created_at: string
}

export interface Pagination {
  page: number
  pageSize: number
  total: number
  totalPages: number
}

export interface PaymentsResponse {
  data: Payment[]
  pagination: Pagination
}

export interface PaymentsQuery extends AnalyticsFilters {
  page: number
  pageSize: number
}
