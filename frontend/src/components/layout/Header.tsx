import PublicOutlinedIcon from '@mui/icons-material/PublicOutlined'
import AppBar from '@mui/material/AppBar'
import Stack from '@mui/material/Stack'
import Toolbar from '@mui/material/Toolbar'
import Typography from '@mui/material/Typography'

/** Top app bar. Purely structural — no data, no navigation logic. */
export function Header() {
  return (
    <AppBar position="fixed" color="inherit" sx={{ zIndex: (theme) => theme.zIndex.drawer + 1 }}>
      <Toolbar>
        <Stack direction="row" spacing={1.5} sx={{ alignItems: 'center' }}>
          <PublicOutlinedIcon color="primary" />
          <Typography variant="h1" component="h1" sx={{ fontSize: '1.125rem' }}>
            Cross-Border Payments Analytics
          </Typography>
        </Stack>
      </Toolbar>
    </AppBar>
  )
}
