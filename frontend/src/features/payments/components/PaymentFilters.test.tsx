import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { EMPTY_FILTERS } from '../../analytics/types'
import { PaymentFilters } from './PaymentFilters'

describe('PaymentFilters', () => {
  it('reports a filter change without owning its own state', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<PaymentFilters value={EMPTY_FILTERS} onChange={onChange} />)

    await user.click(screen.getByLabelText('Provider'))
    await user.click(await screen.findByRole('option', { name: 'provider_a' }))

    expect(onChange).toHaveBeenCalledWith({ provider: 'provider_a' })
  })

  it('clears all filters via the Clear filters action', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<PaymentFilters value={{ provider: 'provider_a', status: 'completed' }} onChange={onChange} />)

    await user.click(screen.getByRole('button', { name: /clear filters/i }))

    expect(onChange).toHaveBeenCalledWith(EMPTY_FILTERS)
  })

  it('disables the clear action when no filters are active', () => {
    render(<PaymentFilters value={EMPTY_FILTERS} onChange={vi.fn()} />)

    expect(screen.getByRole('button', { name: /clear filters/i })).toBeDisabled()
  })
})
