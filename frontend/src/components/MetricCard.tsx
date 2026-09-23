import Box from '@mui/material/Box'
import Paper from '@mui/material/Paper'
import Skeleton from '@mui/material/Skeleton'
import type { SvgIconProps } from '@mui/material/SvgIcon'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import type { ComponentType } from 'react'

export interface MetricCardProps {
  label: string
  value: string
  icon?: ComponentType<SvgIconProps>
  /** Renders a skeleton in place of `value` — used while the metric's
   * query is loading, so the KPI grid keeps its layout instead of
   * popping in. */
  loading?: boolean
}

/** A single KPI tile (e.g. "Total Volume"). Purely presentational — it
 * has no idea where `value` came from, real data or otherwise. */
export function MetricCard({ label, value, icon: Icon, loading = false }: MetricCardProps) {
  return (
    <Paper
      variant="outlined"
      sx={{ p: 2.5, height: '100%', display: 'flex', flexDirection: 'column', gap: 1 }}
    >
      <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between' }}>
        <Typography variant="body2" color="text.secondary">
          {label}
        </Typography>
        {Icon ? <Icon fontSize="small" color="action" /> : null}
      </Stack>
      <Box>
        {loading ? (
          <Skeleton variant="text" width="70%" height={40} />
        ) : (
          <Typography variant="h4" component="p" sx={{ fontWeight: 700 }}>
            {value}
          </Typography>
        )}
      </Box>
    </Paper>
  )
}
