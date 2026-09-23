import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import { useState } from 'react'

import type { AnalyticsFilters } from '../features/analytics/types'
import { EMPTY_FILTERS } from '../features/analytics/types'
import { PaymentFilters } from '../features/payments/components/PaymentFilters'
import { PaymentsTable } from '../features/payments/components/PaymentsTable'
import { usePayments } from '../features/payments/hooks/usePayments'

const PAGE_SIZE = 20

export function PaymentsPage() {
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
        Payments
      </Typography>
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
