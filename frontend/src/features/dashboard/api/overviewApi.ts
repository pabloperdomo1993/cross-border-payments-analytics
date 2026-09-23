import type { CorridorVolume, ProviderStats } from '../../analytics/types'
import type { Overview } from '../types'

/**
 * Pure computation: no /overview endpoint exists on analytics-service,
 * so the four KPIs are derived from two endpoints that DO exist and
 * are already being queried for other parts of the dashboard —
 * corridors (for volume/transaction counts) and providers (for
 * completed/failed counts, i.e. success rate). See
 * hooks/useOverview.ts for how this is wired to real queries (not
 * mock data).
 */
export function computeOverview(corridors: CorridorVolume[], providers: ProviderStats[]): Overview {
  const totalVolume = corridors.reduce((sum, c) => sum + Number.parseFloat(c.total_volume), 0)
  const totalTransactionsFromCorridors = corridors.reduce((sum, c) => sum + c.transaction_count, 0)

  const totalTransactions = providers.reduce((sum, p) => sum + p.total_transactions, 0)
  const totalCompleted = providers.reduce((sum, p) => sum + p.completed_transactions, 0)

  // Prefer the providers-derived transaction count for the KPI (it's
  // what success rate is computed against); corridors' own count is
  // used only as a fallback if providers data isn't available yet, so
  // the two numbers never visibly disagree once both have loaded.
  const transactionCount = totalTransactions || totalTransactionsFromCorridors

  return {
    totalVolume,
    totalTransactions: transactionCount,
    successRate: transactionCount > 0 ? (totalCompleted / totalTransactions) * 100 : 0,
    averagePayment: transactionCount > 0 ? totalVolume / transactionCount : 0,
  }
}
