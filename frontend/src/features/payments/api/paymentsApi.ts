// Real implementation, written against the conceptual
// `GET /api/v1/payments?page=&pageSize=&...` contract. Not wired up yet
// — payments-service doesn't implement this endpoint today (only
// POST /api/v1/transactions and GET /api/v1/transactions/{id} exist).
// See mockPaymentsApi.ts for what's actually used right now, and
// hooks/usePayments.ts for the single line that would need to change
// to switch over once the backend adds this endpoint.
import { paymentsApiClient, type RequestOptions } from '../../../lib/apiClient'
import type { PaymentsQuery, PaymentsResponse } from '../types'

function toQuery(query: PaymentsQuery): RequestOptions['query'] {
  return {
    page: query.page,
    pageSize: query.pageSize,
    from: query.from,
    to: query.to,
    source_country: query.sourceCountry,
    destination_country: query.destinationCountry,
    source_currency: query.sourceCurrency,
    destination_currency: query.destinationCurrency,
    provider: query.provider,
    status: query.status,
  }
}

export function fetchPayments(query: PaymentsQuery, signal?: AbortSignal): Promise<PaymentsResponse> {
  return paymentsApiClient.get<PaymentsResponse>('/api/v1/payments', { query: toQuery(query), signal })
}
