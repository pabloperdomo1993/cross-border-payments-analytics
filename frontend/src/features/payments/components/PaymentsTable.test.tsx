import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import type { Payment } from '../types'
import { PaymentsTable } from './PaymentsTable'

const PAGINATION = { page: 1, pageSize: 20, total: 0, totalPages: 0 }

const SAMPLE_PAYMENT: Payment = {
  id: 'txn_123',
  source_country: 'CO',
  destination_country: 'US',
  source_currency: 'COP',
  destination_currency: 'USD',
  provider: 'provider_a',
  source_amount: '1500.00',
  status: 'completed',
  created_at: '2026-01-15T10:00:00Z',
}

describe('PaymentsTable', () => {
  it('shows an empty state when there are no payments', () => {
    render(
      <PaymentsTable
        payments={[]}
        pagination={PAGINATION}
        onPageChange={vi.fn()}
        onPageSizeChange={vi.fn()}
      />,
    )

    expect(screen.getByText('No payments found.')).toBeInTheDocument()
    expect(screen.getByText('Try changing the current filters.')).toBeInTheDocument()
  })

  it('renders payment rows with corridor and status', () => {
    render(
      <PaymentsTable
        payments={[SAMPLE_PAYMENT]}
        pagination={{ ...PAGINATION, total: 1, totalPages: 1 }}
        onPageChange={vi.fn()}
        onPageSizeChange={vi.fn()}
      />,
    )

    expect(screen.getByText('txn_123')).toBeInTheDocument()
    expect(screen.getByText('CO → US')).toBeInTheDocument()
    expect(screen.getByText('completed')).toBeInTheDocument()
  })
})
