import Alert from '@mui/material/Alert'
import AlertTitle from '@mui/material/AlertTitle'
import Button from '@mui/material/Button'
import Stack from '@mui/material/Stack'

interface ErrorStateProps {
  title?: string
  message: string
  onRetry?: () => void
}

/** Shown whenever a query fails. Always says what went wrong; offers
 * Retry whenever the caller can meaningfully retry (i.e. TanStack
 * Query's `refetch`). */
export function ErrorState({ title = 'Something went wrong', message, onRetry }: ErrorStateProps) {
  return (
    <Alert
      severity="error"
      action={
        onRetry ? (
          <Button color="inherit" size="small" onClick={onRetry}>
            Retry
          </Button>
        ) : undefined
      }
    >
      <Stack spacing={0.25}>
        <AlertTitle>{title}</AlertTitle>
        {message}
      </Stack>
    </Alert>
  )
}
