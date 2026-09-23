import { keepPreviousData, useQuery } from '@tanstack/react-query'

import { mockFetchPayments } from '../api/mockPaymentsApi'
import type { PaymentsQuery } from '../types'

// mockFetchPayments -> fetchPayments (from ../api/paymentsApi) is the
// entire change needed once payments-service implements
// GET /api/v1/payments — nothing else in this file, or any component
// using this hook, needs to change.
export function usePayments(query: PaymentsQuery) {
  return useQuery({
    queryKey: ['payments', 'list', query],
    queryFn: ({ signal }) => mockFetchPayments(query, signal),
    // Keep showing the previous page's rows while the next page loads,
    // instead of flashing a loading state on every pagination click.
    placeholderData: keepPreviousData,
  })
}
