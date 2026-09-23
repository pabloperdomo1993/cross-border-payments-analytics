import ClearIcon from '@mui/icons-material/Clear'
import Button from '@mui/material/Button'
import Grid from '@mui/material/Grid'
import MenuItem from '@mui/material/MenuItem'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'

import type { AnalyticsFilters } from '../../analytics/types'
import { EMPTY_FILTERS } from '../../analytics/types'

const COUNTRIES = ['CO', 'US', 'MX', 'BR', 'AR', 'CL']
const CURRENCIES = ['COP', 'USD', 'MXN', 'BRL', 'ARS', 'CLP']
const PROVIDERS = ['provider_a', 'provider_b', 'provider_c']
const STATUSES: NonNullable<AnalyticsFilters['status']>[] = ['pending', 'completed', 'failed']

export interface PaymentFiltersProps {
  value: AnalyticsFilters
  onChange: (next: AnalyticsFilters) => void
}

/**
 * Controlled filter bar shared by the dashboard, payments, and
 * analytics pages. It never owns filter state itself — the parent page
 * does — so the same filters can drive a table on one page and a chart
 * on another without duplicating state.
 */
export function PaymentFilters({ value, onChange }: PaymentFiltersProps) {
  function set<K extends keyof AnalyticsFilters>(key: K, next: AnalyticsFilters[K]) {
    onChange({ ...value, [key]: next || undefined })
  }

  const hasActiveFilters = Object.values(value).some((v) => v !== undefined && v !== '')

  return (
    <Paper variant="outlined" sx={{ p: 2 }}>
      <Grid container spacing={2} sx={{ alignItems: 'center' }}>
        <Grid size={{ xs: 6, sm: 4, md: 2 }}>
          <TextField
            select
            fullWidth
            size="small"
            label="Source country"
            value={value.sourceCountry ?? ''}
            onChange={(e) => set('sourceCountry', e.target.value)}
          >
            <MenuItem value="">Any</MenuItem>
            {COUNTRIES.map((c) => (
              <MenuItem key={c} value={c}>
                {c}
              </MenuItem>
            ))}
          </TextField>
        </Grid>
        <Grid size={{ xs: 6, sm: 4, md: 2 }}>
          <TextField
            select
            fullWidth
            size="small"
            label="Destination country"
            value={value.destinationCountry ?? ''}
            onChange={(e) => set('destinationCountry', e.target.value)}
          >
            <MenuItem value="">Any</MenuItem>
            {COUNTRIES.map((c) => (
              <MenuItem key={c} value={c}>
                {c}
              </MenuItem>
            ))}
          </TextField>
        </Grid>
        <Grid size={{ xs: 6, sm: 4, md: 2 }}>
          <TextField
            select
            fullWidth
            size="small"
            label="Currency"
            value={value.sourceCurrency ?? ''}
            onChange={(e) => set('sourceCurrency', e.target.value)}
          >
            <MenuItem value="">Any</MenuItem>
            {CURRENCIES.map((c) => (
              <MenuItem key={c} value={c}>
                {c}
              </MenuItem>
            ))}
          </TextField>
        </Grid>
        <Grid size={{ xs: 6, sm: 4, md: 2 }}>
          <TextField
            select
            fullWidth
            size="small"
            label="Provider"
            value={value.provider ?? ''}
            onChange={(e) => set('provider', e.target.value)}
          >
            <MenuItem value="">Any</MenuItem>
            {PROVIDERS.map((p) => (
              <MenuItem key={p} value={p}>
                {p}
              </MenuItem>
            ))}
          </TextField>
        </Grid>
        <Grid size={{ xs: 6, sm: 4, md: 2 }}>
          <TextField
            select
            fullWidth
            size="small"
            label="Status"
            value={value.status ?? ''}
            onChange={(e) => set('status', e.target.value as AnalyticsFilters['status'])}
          >
            <MenuItem value="">Any</MenuItem>
            {STATUSES.map((s) => (
              <MenuItem key={s} value={s}>
                {s}
              </MenuItem>
            ))}
          </TextField>
        </Grid>
        <Grid size={{ xs: 12, sm: 4, md: 2 }}>
          <Stack direction="row" sx={{ justifyContent: 'flex-end' }}>
            <Button
              size="small"
              startIcon={<ClearIcon fontSize="small" />}
              onClick={() => onChange(EMPTY_FILTERS)}
              disabled={!hasActiveFilters}
            >
              Clear filters
            </Button>
          </Stack>
        </Grid>
        <Grid size={{ xs: 6, sm: 4, md: 3 }}>
          <TextField
            fullWidth
            size="small"
            type="date"
            label="From"
            slotProps={{ inputLabel: { shrink: true } }}
            value={value.from ?? ''}
            onChange={(e) => set('from', e.target.value)}
          />
        </Grid>
        <Grid size={{ xs: 6, sm: 4, md: 3 }}>
          <TextField
            fullWidth
            size="small"
            type="date"
            label="To"
            slotProps={{ inputLabel: { shrink: true } }}
            value={value.to ?? ''}
            onChange={(e) => set('to', e.target.value)}
          />
        </Grid>
      </Grid>
    </Paper>
  )
}
