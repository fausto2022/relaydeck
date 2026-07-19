import { lazy, StrictMode, Suspense } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Route, Routes } from 'react-router-dom'
import '@fontsource-variable/geist'
import '@fontsource-variable/geist-mono'
import { ThemeProvider } from '@/components/theme-provider'
import { AuthProvider } from '@/lib/auth-context'
import { RefreshProvider } from '@/lib/refresh-context'
import { AddChannelProvider } from '@/lib/add-channel-context'
import { AuthGate } from '@/components/auth/auth-gate'
import { AppShell } from '@/components/app-shell'
import { AppErrorBoundary } from '@/components/app-error-boundary'
import { Toaster } from '@/components/ui/sonner'
import { Spinner } from '@/components/ui/spinner'
import '@/app/globals.css'

const DashboardPage = lazy(() => import('@/app/page'))
const CaptchaPage = lazy(() => import('@/app/captcha-page'))
const NotificationsPage = lazy(() => import('@/app/notifications-page'))
const SettingsPage = lazy(() => import('@/app/settings-page'))
const MainStationPage = lazy(() => import('@/app/main-station-page'))

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ThemeProvider attribute="class" defaultTheme="light" enableSystem disableTransitionOnChange>
      <AppErrorBoundary>
        <AuthProvider>
          <AuthGate>
            <RefreshProvider>
              <BrowserRouter>
                <AddChannelProvider>
                  <Suspense fallback={<div className="flex min-h-72 items-center justify-center"><Spinner /></div>}>
                    <Routes>
                      <Route element={<AppShell />}>
                        <Route index element={<DashboardPage />} />
                        <Route path="captcha" element={<CaptchaPage />} />
                        <Route path="notifications" element={<NotificationsPage />} />
                        <Route path="main-station" element={<MainStationPage />} />
                        <Route path="settings" element={<SettingsPage />} />
                      </Route>
                    </Routes>
                  </Suspense>
                </AddChannelProvider>
              </BrowserRouter>
            </RefreshProvider>
            <Toaster richColors closeButton position="top-right" />
          </AuthGate>
        </AuthProvider>
      </AppErrorBoundary>
    </ThemeProvider>
  </StrictMode>,
)
