import Chip from '@mui/material/Chip'
import Paper from '@mui/material/Paper'
import Skeleton from '@mui/material/Skeleton'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableContainer from '@mui/material/TableContainer'
import TableHead from '@mui/material/TableHead'
import TablePagination from '@mui/material/TablePagination'
import TableRow from '@mui/material/TableRow'

import { EmptyState } from '../../../components/feedback/EmptyState'
import type { Payment, Pagination } from '../types'

const STATUS_COLOR: Record<Payment['status'], 'success' | 'warning' | 'error'> = {
  completed: 'success',
  pending: 'warning',
  failed: 'error',
}

function formatAmount(payment: Payment): string {
  const amount = Number.parseFloat(payment.source_amount)
  return `${amount.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })} ${payment.source_currency}`
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

export interface PaymentsTableProps {
  payments: Payment[]
  pagination: Pagination
  loading?: boolean
  onPageChange: (page: number) => void
  onPageSizeChange: (pageSize: number) => void
}

/** Server-paginated payments table. `pagination` always reflects the
 * server's (or mock's) page/pageSize/total — this component never
 * re-derives pagination from `payments.length`. */
export function PaymentsTable({
  payments,
  pagination,
  loading = false,
  onPageChange,
  onPageSizeChange,
}: PaymentsTableProps) {
  if (!loading && payments.length === 0) {
    return (
      <Paper variant="outlined">
        <EmptyState title="No payments found." description="Try changing the current filters." />
      </Paper>
    )
  }

  return (
    <Paper variant="outlined">
      <TableContainer>
        <Table size="small" aria-label="Payments">
          <TableHead>
            <TableRow>
              <TableCell>Payment ID</TableCell>
              <TableCell>Corridor</TableCell>
              <TableCell>Currency pair</TableCell>
              <TableCell>Provider</TableCell>
              <TableCell align="right">Amount</TableCell>
              <TableCell>Status</TableCell>
              <TableCell>Created</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {loading
              ? Array.from({ length: pagination.pageSize }, (_, i) => (
                  <TableRow key={i}>
                    {Array.from({ length: 7 }, (__, j) => (
                      <TableCell key={j}>
                        <Skeleton variant="text" />
                      </TableCell>
                    ))}
                  </TableRow>
                ))
              : payments.map((payment) => (
                  <TableRow key={payment.id} hover>
                    <TableCell sx={{ fontFamily: 'monospace', fontSize: '0.8125rem' }}>
                      {payment.id}
                    </TableCell>
                    <TableCell>
                      {payment.source_country} → {payment.destination_country}
                    </TableCell>
                    <TableCell>
                      {payment.source_currency}/{payment.destination_currency}
                    </TableCell>
                    <TableCell>{payment.provider}</TableCell>
                    <TableCell align="right">{formatAmount(payment)}</TableCell>
                    <TableCell>
                      <Chip
                        label={payment.status}
                        color={STATUS_COLOR[payment.status]}
                        size="small"
                        variant="outlined"
                      />
                    </TableCell>
                    <TableCell>{formatDate(payment.created_at)}</TableCell>
                  </TableRow>
                ))}
          </TableBody>
        </Table>
      </TableContainer>
      <TablePagination
        component="div"
        count={pagination.total}
        page={pagination.page - 1}
        rowsPerPage={pagination.pageSize}
        rowsPerPageOptions={[10, 20, 50]}
        onPageChange={(_, newPage) => onPageChange(newPage + 1)}
        onRowsPerPageChange={(e) => onPageSizeChange(Number.parseInt(e.target.value, 10))}
      />
    </Paper>
  )
}
