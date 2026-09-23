/** Derived client-side from analytics-service's real /corridors and
 * /providers endpoints — see api/overviewApi.ts. There is no backend
 * /overview endpoint; this shape matches the one the task's own spec
 * describes, but it's computed here, not mocked. */
export interface Overview {
  totalVolume: number
  totalTransactions: number
  successRate: number
  averagePayment: number
}
