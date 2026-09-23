// Centralized, typed HTTP client for both backend APIs. Components and
// hooks never see a URL, a fetch call, or a status code directly — they
// only see `paymentsApiClient`/`analyticsApiClient` and typed results or
// a thrown `ApiError`.
//
// Two backends, two base URLs: this project has separate
// payments-service and analytics-service HTTP APIs (see
// docs/architecture.md), so there are two small clients below rather
// than one generic one — each still shares the same request/error
// handling implementation.

/** Thrown for any non-2xx response, or a network failure. */
export class ApiError extends Error {
  readonly status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

// Vite bakes these in at build time. The fallbacks match the ports
// docker-compose.yml already publishes, so the app works out of the
// box with no .env file present — see frontend/.env.example.
const PAYMENTS_API_URL: string =
  import.meta.env.VITE_PAYMENTS_API_URL ?? 'http://localhost:8080'
const ANALYTICS_API_URL: string =
  import.meta.env.VITE_ANALYTICS_API_URL ?? 'http://localhost:8082'

/** Matches the {"error":{"code","message","fields"}} envelope every
 * backend service in this repo returns for non-2xx responses. */
interface ApiErrorBody {
  error?: {
    code?: string
    message?: string
    fields?: Record<string, string>
  }
}

export interface RequestOptions {
  /** Query parameters; `undefined`/`''` values are omitted, not sent as empty strings. */
  query?: Record<string, string | number | undefined>
  signal?: AbortSignal
}

function buildUrl(baseUrl: string, path: string, query?: RequestOptions['query']): string {
  const url = new URL(path, baseUrl)
  if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined && value !== '') {
        url.searchParams.set(key, String(value))
      }
    }
  }
  return url.toString()
}

async function request<T>(baseUrl: string, path: string, options: RequestOptions = {}): Promise<T> {
  const url = buildUrl(baseUrl, path, options.query)

  let response: Response
  try {
    response = await fetch(url, { signal: options.signal })
  } catch (error) {
    // Let AbortError propagate as-is — TanStack Query treats it
    // specially (a cancelled query, not a failed one).
    if (error instanceof DOMException && error.name === 'AbortError') {
      throw error
    }
    throw new ApiError(`Could not reach ${url}. Is the service running?`, 0)
  }

  if (!response.ok) {
    let message = `Request to ${url} failed with status ${response.status}`
    try {
      const body = (await response.json()) as ApiErrorBody
      if (body.error?.message) {
        message = body.error.message
      }
    } catch {
      // Response body wasn't JSON (or was empty) — keep the generic message.
    }
    throw new ApiError(message, response.status)
  }

  return (await response.json()) as T
}

export const paymentsApiClient = {
  get: <T>(path: string, options?: RequestOptions): Promise<T> =>
    request<T>(PAYMENTS_API_URL, path, options),
}

export const analyticsApiClient = {
  get: <T>(path: string, options?: RequestOptions): Promise<T> =>
    request<T>(ANALYTICS_API_URL, path, options),
}
