import InboxOutlinedIcon from '@mui/icons-material/InboxOutlined'
import Box from '@mui/material/Box'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'

interface EmptyStateProps {
  title: string
  description?: string
}

/** Shown when a query succeeds but returns no rows — distinct from
 * ErrorState (nothing failed) and LoadingState (nothing is pending). */
export function EmptyState({ title, description }: EmptyStateProps) {
  return (
    <Box sx={{ display: 'flex', justifyContent: 'center', py: 6 }}>
      <Stack spacing={1} sx={{ alignItems: 'center', textAlign: 'center', maxWidth: 320 }}>
        <InboxOutlinedIcon sx={{ fontSize: 40, color: 'text.disabled' }} />
        <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
          {title}
        </Typography>
        {description ? (
          <Typography variant="body2" color="text.secondary">
            {description}
          </Typography>
        ) : null}
      </Stack>
    </Box>
  )
}
