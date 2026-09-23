import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import { useState } from 'react'

import { CorridorVolumeChart } from '../features/dashboard/components/CorridorVolumeChart'
import { PaymentVolumeChart } from '../features/dashboard/components/PaymentVolumeChart'
import type { AnalyticsFilters } from '../features/analytics/types'
import { EMPTY_FILTERS } from '../features/analytics/types'
import { PaymentFilters } from '../features/payments/components/PaymentFilters'

export function AnalyticsPage() {
  const [filters, setFilters] = useState<AnalyticsFilters>(EMPTY_FILTERS)

  return (
    <Stack spacing={3}>
      <Typography variant="h5" sx={{ fontWeight: 700 }}>
        Analytics
      </Typography>
      <PaymentFilters value={filters} onChange={setFilters} />
      <PaymentVolumeChart filters={filters} />
      <CorridorVolumeChart filters={filters} />
    </Stack>
  )
}
