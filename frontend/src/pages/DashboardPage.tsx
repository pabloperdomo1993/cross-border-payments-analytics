import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import { useState } from 'react'

import { CorridorVolumeChart } from '../features/dashboard/components/CorridorVolumeChart'
import { KpiSection } from '../features/dashboard/components/KpiSection'
import { PaymentVolumeChart } from '../features/dashboard/components/PaymentVolumeChart'
import { PaymentFilters } from '../features/payments/components/PaymentFilters'
import { PaymentsTable } from '../features/payments/components/PaymentsTable'
import { usePayments } from '../features/payments/hooks/usePayments'
import type { AnalyticsFilters } from '../features/analytics/types'
import { EMPTY_FILTERS } from '../features/analytics/types'

const PAGE_SIZE = 10

export function DashboardPage() {
  const [filters, setFilters] = useState<AnalyticsFilters>(EMPTY_FILTERS)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(PAGE_SIZE)

  const { data, isLoading } = usePayments({ ...filters, page, pageSize })

  function handleFiltersChange(next: AnalyticsFilters) {
    setFilters(next)
    setPage(1)
  }

  return (
    <Stack spacing={3}>
      <Typography variant="h5" sx={{ fontWeight: 700 }}>
        Dashboard
      </Typography>

      <KpiSection filters={filters} />
      <PaymentVolumeChart filters={filters} />
      <CorridorVolumeChart filters={filters} />
      <PaymentFilters value={filters} onChange={handleFiltersChange} />
      <PaymentsTable
        payments={data?.data ?? []}
        pagination={data?.pagination ?? { page, pageSize, total: 0, totalPages: 0 }}
        loading={isLoading}
        onPageChange={setPage}
        onPageSizeChange={(size) => {
          setPageSize(size)
          setPage(1)
        }}
      />
    </Stack>
  )
}
