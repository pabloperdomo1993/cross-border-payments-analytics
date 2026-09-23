# Frontend — Cross-Border Payments Analytics

A React + TypeScript dashboard for the cross-border payments platform: KPI cards, a payment volume chart, a corridor volume chart, and a filterable, paginated payments table.

## Stack

React 19, TypeScript, Vite, Material UI (+ MUI X Charts), React Router, TanStack Query. No Redux/Zustand — server state lives entirely in the TanStack Query cache; the only client state is filter/pagination selections held in each page component.

## Running

```bash
npm install
npm run dev       # http://localhost:5173, proxies nothing — calls the two APIs directly
npm run build      # tsc -b && vite build
npm run lint
npm run test       # vitest run
npm run test:watch
```

Or via Docker, from the repo root: `docker compose up --build frontend` (serves the production build through nginx on `:5173`).

## How the frontend talks to the backend

There are two backend HTTP APIs, and the frontend never hardcodes their URLs — every request goes through the typed clients in [src/lib/apiClient.ts](src/lib/apiClient.ts):

- **payments-service** (`VITE_PAYMENTS_API_URL`, default `http://localhost:8080`)
- **analytics-service** (`VITE_ANALYTICS_API_URL`, default `http://localhost:8082`)

Copy [.env.example](.env.example) to `.env` to override these. Vite bakes `VITE_*` vars in at **build** time, and since the dashboard runs entirely in the browser (dev server, `vite preview`, or the nginx-served Docker build), the values that matter are always the **host-published** ports — never a Docker service name — regardless of how the frontend itself is served.

### Endpoints actually used

| Feature | Endpoint | Status |
|---|---|---|
| KPI cards, corridor chart | `GET /api/v1/analytics/corridors` (analytics-service) | Real |
| KPI cards (success rate) | `GET /api/v1/analytics/providers` (analytics-service) | Real |
| Payment volume chart | `GET /api/v1/analytics/timeseries` (analytics-service) | Real |
| Payments table | `GET /api/v1/payments` (payments-service) | **Mocked** — see below |

analytics-service has no `/overview` endpoint, so the four KPIs (total volume, total transactions, success rate, average payment) are computed client-side from the corridors + providers responses — see `src/features/dashboard/api/overviewApi.ts`. This needed no mocking at all.

payments-service has no paginated list endpoint today (only `POST /api/v1/transactions` and `GET /api/v1/transactions/{id}`), so the payments table is served by a deterministic, seeded in-memory generator (`src/features/payments/api/mockPaymentsApi.ts`) shaped exactly like payments-service's real transaction response. Swapping to the real endpoint once it exists is a one-line change in `src/features/payments/hooks/usePayments.ts` (`mockFetchPayments` → `fetchPayments`); no component changes needed.

## Testing

Vitest + React Testing Library + jest-dom + user-event. Tests focus on user-visible behavior (rendering, loading/error/empty states, filter interactions) rather than implementation details — see `src/**/*.test.tsx`.
