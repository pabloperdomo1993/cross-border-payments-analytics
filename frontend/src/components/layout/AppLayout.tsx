import BarChartOutlinedIcon from '@mui/icons-material/BarChartOutlined'
import DashboardOutlinedIcon from '@mui/icons-material/DashboardOutlined'
import PaymentsOutlinedIcon from '@mui/icons-material/PaymentsOutlined'
import Box from '@mui/material/Box'
import Drawer from '@mui/material/Drawer'
import List from '@mui/material/List'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Toolbar from '@mui/material/Toolbar'
import type { ReactNode } from 'react'
import { NavLink } from 'react-router-dom'

import { Header } from './Header'

const DRAWER_WIDTH = 220

const NAV_ITEMS = [
  { label: 'Dashboard', to: '/', icon: DashboardOutlinedIcon },
  { label: 'Payments', to: '/payments', icon: PaymentsOutlinedIcon },
  { label: 'Analytics', to: '/analytics', icon: BarChartOutlinedIcon },
]

interface AppLayoutProps {
  children: ReactNode
}

/** The overall app chrome: fixed header + left nav + main content
 * area. Every page renders inside this. */
export function AppLayout({ children }: AppLayoutProps) {
  return (
    <Box sx={{ display: 'flex' }}>
      <Header />
      <Drawer
        variant="permanent"
        sx={{
          width: DRAWER_WIDTH,
          flexShrink: 0,
          [`& .MuiDrawer-paper`]: { width: DRAWER_WIDTH, boxSizing: 'border-box' },
        }}
      >
        <Toolbar />
        <List sx={{ px: 1 }}>
          {NAV_ITEMS.map(({ label, to, icon: Icon }) => (
            <ListItemButton
              key={to}
              component={NavLink}
              to={to}
              end={to === '/'}
              sx={{
                borderRadius: 1,
                mb: 0.5,
                '&.active': {
                  bgcolor: 'action.selected',
                  fontWeight: 600,
                },
              }}
            >
              <ListItemIcon sx={{ minWidth: 36 }}>
                <Icon fontSize="small" />
              </ListItemIcon>
              <ListItemText primary={label} />
            </ListItemButton>
          ))}
        </List>
      </Drawer>
      <Box component="main" sx={{ flexGrow: 1, bgcolor: 'background.default', minHeight: '100vh' }}>
        <Toolbar />
        <Box sx={{ p: { xs: 2, md: 3 } }}>{children}</Box>
      </Box>
    </Box>
  )
}
