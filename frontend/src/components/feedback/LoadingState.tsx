import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'

interface LoadingStateProps {
  label?: string
  minHeight?: number | string
}

/** Generic async-loading placeholder. Use `Skeleton` directly inside a
 * component when the final layout is known (e.g. a table); use this
 * when a whole section is loading and there's nothing more specific to
 * show yet. */
export function LoadingState({ label = 'Loading…', minHeight = 160 }: LoadingStateProps) {
  return (
    <Box
      sx={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        minHeight,
        width: '100%',
      }}
    >
      <Stack spacing={1.5} sx={{ alignItems: 'center' }}>
        <CircularProgress size={28} />
        <Typography variant="body2" color="text.secondary">
          {label}
        </Typography>
      </Stack>
    </Box>
  )
}
