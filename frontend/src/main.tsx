import { lazy, StrictMode, Suspense } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom'
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
const SettingsPage = lazy(() => import('@/app/settings-page'))
const MainStationPage = lazy(() => import('@/app/main-station-page'))
const ChannelsPage = lazy(() => import('@/app/channels-page'))
const RatesPage = lazy(() => import('@/app/rates-page'))

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
                        <Route path="channels" element={<ChannelsPage />} />
                        <Route path="rates" element={<RatesPage />} />
                        <Route path="captcha" element={<Navigate to="/settings?tab=captcha" replace />} />
                        <Route path="notifications" element={<Navigate to="/settings?tab=notifications" replace />} />
                        <Route path="main-station" element={<MainStationPage />} />
                        <Route path="settings" element={<SettingsPage />} />
                        <Route path="*" element={<Navigate to="/" replace />} />
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
