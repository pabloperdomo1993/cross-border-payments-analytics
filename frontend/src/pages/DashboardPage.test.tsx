import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { useOverview } from '../features/dashboard/hooks/useOverview'
import { DashboardPage } from './DashboardPage'

const refetch = vi.fn()

vi.mock('../features/dashboard/hooks/useOverview', () => ({
  useOverview: vi.fn(),
}))
vi.mock('../features/dashboard/hooks/usePaymentVolume', () => ({
  usePaymentVolume: vi.fn(() => ({ data: [], isLoading: false, isError: false, error: null, refetch })),
}))
vi.mock('../features/analytics/hooks/useAnalyticsQueries', () => ({
  useCorridors: vi.fn(() => ({ data: [], isLoading: false, isError: false, error: null, refetch })),
}))
vi.mock('../features/payments/hooks/usePayments', () => ({
  usePayments: vi.fn(() => ({
    data: { data: [], pagination: { page: 1, pageSize: 10, total: 0, totalPages: 0 } },
    isLoading: false,
  })),
}))

describe('DashboardPage', () => {
  it('shows loading skeletons for the KPIs while data is pending', () => {
    vi.mocked(useOverview).mockReturnValue({
      data: undefined,
      isLoading: true,
      isError: false,
      error: null,
      refetch,
    })

    render(<DashboardPage />)

    expect(screen.getAllByText('Total Volume').length).toBeGreaterThan(0)
    // MetricCard renders a Skeleton (no accessible text) in place of the value while loading.
    expect(screen.queryByText('$0')).not.toBeInTheDocument()
  })

  it('shows an error state with a retry action when the KPI query fails', () => {
    vi.mocked(useOverview).mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new Error('Network unreachable'),
      refetch,
    })

    render(<DashboardPage />)

    expect(screen.getByText('Network unreachable')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /retry/i })).toBeInTheDocument()
  })

  it('renders the KPI values once the overview query succeeds', () => {
    vi.mocked(useOverview).mockReturnValue({
      data: { totalVolume: 125000, totalTransactions: 500, successRate: 92.5, averagePayment: 250 },
      isLoading: false,
      isError: false,
      error: null,
      refetch,
    })

    render(<DashboardPage />)

    expect(screen.getByText('92.5%')).toBeInTheDocument()
  })
})
