import AccountBalanceWalletOutlinedIcon from '@mui/icons-material/AccountBalanceWalletOutlined'
import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutlined'
import ReceiptLongOutlinedIcon from '@mui/icons-material/ReceiptLongOutlined'
import TrendingUpOutlinedIcon from '@mui/icons-material/TrendingUpOutlined'
import Grid from '@mui/material/Grid'

import { ErrorState } from '../../../components/feedback/ErrorState'
import { MetricCard } from '../../../components/MetricCard'
import type { AnalyticsFilters } from '../../analytics/types'
import { useOverview } from '../hooks/useOverview'

const currencyFormatter = new Intl.NumberFormat(undefined, {
  style: 'currency',
  currency: 'USD',
  maximumFractionDigits: 0,
})
const integerFormatter = new Intl.NumberFormat(undefined)

export interface KpiSectionProps {
  filters: AnalyticsFilters
}

/** The four top-level KPI cards. Values are derived from real
 * analytics-service data (see hooks/useOverview.ts) — there is no
 * mock data involved here. */
export function KpiSection({ filters }: KpiSectionProps) {
  const { data, isLoading, isError, error, refetch } = useOverview(filters)

  if (isError) {
    return (
      <ErrorState
        title="Could not load KPIs"
        message={error?.message ?? 'Unknown error'}
        onRetry={refetch}
      />
    )
  }

  const cards = [
    {
      label: 'Total Volume',
      value: data ? currencyFormatter.format(data.totalVolume) : '',
      icon: AccountBalanceWalletOutlinedIcon,
    },
    {
      label: 'Total Transactions',
      value: data ? integerFormatter.format(data.totalTransactions) : '',
      icon: ReceiptLongOutlinedIcon,
    },
    {
      label: 'Success Rate',
      value: data ? `${data.successRate.toFixed(1)}%` : '',
      icon: CheckCircleOutlineIcon,
    },
    {
      label: 'Average Payment',
      value: data ? currencyFormatter.format(data.averagePayment) : '',
      icon: TrendingUpOutlinedIcon,
    },
  ]

  return (
    <Grid container spacing={2}>
      {cards.map((card) => (
        <Grid key={card.label} size={{ xs: 12, sm: 6, md: 3 }}>
          <MetricCard label={card.label} value={card.value} icon={card.icon} loading={isLoading} />
        </Grid>
      ))}
    </Grid>
  )
}
