import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { MetricCard } from './MetricCard'

describe('MetricCard', () => {
  it('renders the label and value', () => {
    render(<MetricCard label="Total Volume" value="$1,234" />)

    expect(screen.getByText('Total Volume')).toBeInTheDocument()
    expect(screen.getByText('$1,234')).toBeInTheDocument()
  })

  it('shows a skeleton instead of the value while loading', () => {
    render(<MetricCard label="Total Volume" value="$1,234" loading />)

    expect(screen.getByText('Total Volume')).toBeInTheDocument()
    expect(screen.queryByText('$1,234')).not.toBeInTheDocument()
  })
})
