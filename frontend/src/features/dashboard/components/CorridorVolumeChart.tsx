import Paper from '@mui/material/Paper'
import Typography from '@mui/material/Typography'
import { BarChart } from '@mui/x-charts/BarChart'

import { EmptyState } from '../../../components/feedback/EmptyState'
import { ErrorState } from '../../../components/feedback/ErrorState'
import { LoadingState } from '../../../components/feedback/LoadingState'
import { useCorridors } from '../../analytics/hooks/useAnalyticsQueries'
import type { AnalyticsFilters } from '../../analytics/types'

export interface CorridorVolumeChartProps {
  filters: AnalyticsFilters
}

/** "Volume by Payment Corridor" — reuses the same corridors query the
 * KPI cards already fetch (shared cache entry, no extra request). */
export function CorridorVolumeChart({ filters }: CorridorVolumeChartProps) {
  const { data, isLoading, isError, error, refetch } = useCorridors(filters)

  const top = [...(data ?? [])]
    .sort((a, b) => Number.parseFloat(b.total_volume) - Number.parseFloat(a.total_volume))
    .slice(0, 8)

  return (
    <Paper variant="outlined" sx={{ p: 2.5 }}>
      <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 1 }}>
        Volume by Payment Corridor
      </Typography>
      {isLoading ? (
        <LoadingState label="Loading chart…" minHeight={280} />
      ) : isError ? (
        <ErrorState message={error?.message ?? 'Unknown error'} onRetry={refetch} />
      ) : top.length === 0 ? (
        <EmptyState title="No corridor data for this period." description="Try changing the current filters." />
      ) : (
        <BarChart
          height={280}
          dataset={top.map((c) => ({
            corridor: `${c.source_country}→${c.destination_country}`,
            volume: Number.parseFloat(c.total_volume),
          }))}
          xAxis={[{ dataKey: 'corridor', scaleType: 'band' }]}
          series={[{ dataKey: 'volume', label: 'Volume', color: '#1a2b4c' }]}
          margin={{ left: 70 }}
        />
      )}
    </Paper>
  )
}
