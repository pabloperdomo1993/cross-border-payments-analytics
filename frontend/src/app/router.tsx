import { createBrowserRouter } from 'react-router-dom'

import { AppLayout } from '../components/layout/AppLayout'
import { AnalyticsPage } from '../pages/AnalyticsPage'
import { DashboardPage } from '../pages/DashboardPage'
import { PaymentsPage } from '../pages/PaymentsPage'

export const router = createBrowserRouter([
  {
    path: '/',
    element: (
      <AppLayout>
        <DashboardPage />
      </AppLayout>
    ),
  },
  {
    path: '/payments',
    element: (
      <AppLayout>
        <PaymentsPage />
      </AppLayout>
    ),
  },
  {
    path: '/analytics',
    element: (
      <AppLayout>
        <AnalyticsPage />
      </AppLayout>
    ),
  },
])
