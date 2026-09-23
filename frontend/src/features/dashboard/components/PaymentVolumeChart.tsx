import Paper from '@mui/material/Paper'
import Typography from '@mui/material/Typography'
import { LineChart } from '@mui/x-charts/LineChart'

import { EmptyState } from '../../../components/feedback/EmptyState'
import { ErrorState } from '../../../components/feedback/ErrorState'
import { LoadingState } from '../../../components/feedback/LoadingState'
import type { AnalyticsFilters } from '../../analytics/types'
import { usePaymentVolume } from '../hooks/usePaymentVolume'

export interface PaymentVolumeChartProps {
  filters: AnalyticsFilters
}

/** "Payment Volume Over Time" — the one chart the spec explicitly
 * calls for. Backed by analytics-service's real /timeseries endpoint. */
export function PaymentVolumeChart({ filters }: PaymentVolumeChartProps) {
  const { data, isLoading, isError, error, refetch } = usePaymentVolume(filters)

  return (
    <Paper variant="outlined" sx={{ p: 2.5 }}>
      <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 1 }}>
        Payment Volume Over Time
      </Typography>
      {isLoading ? (
        <LoadingState label="Loading chart…" minHeight={280} />
      ) : isError ? (
        <ErrorState message={error?.message ?? 'Unknown error'} onRetry={refetch} />
      ) : !data || data.length === 0 ? (
        <EmptyState title="No data for this period." description="Try changing the current filters." />
      ) : (
        <LineChart
          height={280}
          dataset={data.map((point) => ({
            date: new Date(point.period_start).toLocaleDateString(undefined, {
              month: 'short',
              day: 'numeric',
            }),
            volume: Number.parseFloat(point.total_volume),
          }))}
          xAxis={[{ dataKey: 'date', scaleType: 'band' }]}
          series={[{ dataKey: 'volume', label: 'Volume', color: '#0f766e' }]}
          margin={{ left: 70 }}
        />
      )}
    </Paper>
  )
}
