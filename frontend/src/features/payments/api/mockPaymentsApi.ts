// Deterministic, in-memory stand-in for a payments list endpoint that
// doesn't exist yet on the backend (see types.ts for why the shape
// matches payments-service's real single-transaction response). Uses
// the same dimensions as the Go `scripts/seed` tool so demo data looks
// consistent with whatever's actually seeded into the real databases.
//
// This is the ONLY file that knows this data is fake. `usePayments`
// imports `mockFetchPayments` today; swapping to a real
// `GET /api/v1/payments` implementation later is a one-line change
// there, not a rewrite of any component.
import type { Payment, PaymentsQuery, PaymentsResponse } from '../types'

const COUNTRIES = ['CO', 'US', 'MX', 'BR', 'AR', 'CL'] as const
const COUNTRY_CURRENCY: Record<string, string> = {
  CO: 'COP',
  US: 'USD',
  MX: 'MXN',
  BR: 'BRL',
  AR: 'ARS',
  CL: 'CLP',
}
const PROVIDERS = ['provider_a', 'provider_b', 'provider_c'] as const
const STATUSES: Payment['status'][] = ['completed', 'completed', 'completed', 'failed', 'pending']

const DATASET_SIZE = 240
const SEED = 42

/** A small, fast, deterministic PRNG (mulberry32) — same seed always
 * produces the same sequence, so the mock dataset is stable across
 * renders/reloads without needing to persist it anywhere. */
function mulberry32(seed: number): () => number {
  let a = seed
  return () => {
    a |= 0
    a = (a + 0x6d2b79f5) | 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

function pick<T>(rng: () => number, items: readonly T[]): T {
  return items[Math.floor(rng() * items.length)]
}

function generateDataset(): Payment[] {
  const rng = mulberry32(SEED)
  const now = Date.now()
  const rows: Payment[] = []

  for (let i = 0; i < DATASET_SIZE; i++) {
    const sourceCountry = pick(rng, COUNTRIES)
    let destinationCountry = pick(rng, COUNTRIES)
    while (destinationCountry === sourceCountry) {
      destinationCountry = pick(rng, COUNTRIES)
    }

    const amount = Math.round((50 + rng() * 9950) * 100) / 100
    const daysAgo = Math.floor(rng() * 60)
    const createdAt = new Date(now - daysAgo * 24 * 60 * 60 * 1000 - Math.floor(rng() * 86_400_000))

    rows.push({
      id: `demo-${String(i + 1).padStart(4, '0')}`,
      source_country: sourceCountry,
      destination_country: destinationCountry,
      source_currency: COUNTRY_CURRENCY[sourceCountry],
      destination_currency: COUNTRY_CURRENCY[destinationCountry],
      provider: pick(rng, PROVIDERS),
      source_amount: amount.toFixed(2),
      status: pick(rng, STATUSES),
      created_at: createdAt.toISOString(),
    })
  }

  return rows.sort((a, b) => b.created_at.localeCompare(a.created_at))
}

// Generated once per page load, not per request — a real API wouldn't
// regenerate its dataset on every call either.
const DATASET = generateDataset()

function matchesQuery(payment: Payment, query: PaymentsQuery): boolean {
  if (query.sourceCountry && payment.source_country !== query.sourceCountry) return false
  if (query.destinationCountry && payment.destination_country !== query.destinationCountry) return false
  if (query.sourceCurrency && payment.source_currency !== query.sourceCurrency) return false
  if (query.destinationCurrency && payment.destination_currency !== query.destinationCurrency) return false
  if (query.provider && payment.provider !== query.provider) return false
  if (query.status && payment.status !== query.status) return false
  if (query.from && payment.created_at < query.from) return false
  if (query.to && payment.created_at > query.to) return false
  return true
}

/** Simulates network latency so loading states are actually visible
 * during local development instead of resolving instantly. */
function delay(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(resolve, ms)
    signal?.addEventListener('abort', () => {
      clearTimeout(timer)
      reject(new DOMException('Aborted', 'AbortError'))
    })
  })
}

export async function mockFetchPayments(query: PaymentsQuery, signal?: AbortSignal): Promise<PaymentsResponse> {
  await delay(250, signal)

  const filtered = DATASET.filter((payment) => matchesQuery(payment, query))
  const total = filtered.length
  const totalPages = Math.max(1, Math.ceil(total / query.pageSize))
  const start = (query.page - 1) * query.pageSize
  const data = filtered.slice(start, start + query.pageSize)

  return {
    data,
    pagination: { page: query.page, pageSize: query.pageSize, total, totalPages },
  }
}
