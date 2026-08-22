"use client"

import { useEffect, useState } from "react"
import { Outlet } from "react-router-dom"
import { AppSidebar } from "@/components/app-sidebar"
import { MonitorHeader } from "@/components/monitor/monitor-header"
import { cn } from "@/lib/utils"

export function AppShell() {
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false)
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false)

  useEffect(() => {
    setSidebarCollapsed(window.localStorage.getItem("relaydeck-sidebar-collapsed") === "true")
    document.title = "管理后台"
  }, [])

  function handleCollapsedChange(collapsed: boolean) {
    setSidebarCollapsed(collapsed)
    window.localStorage.setItem("relaydeck-sidebar-collapsed", String(collapsed))
  }

  return (
    <div className="min-h-dvh min-w-0 bg-background">
      <a
        href="#main-content"
        className="fixed left-3 top-3 z-50 -translate-y-20 rounded-md bg-primary px-3 py-2 text-sm font-medium text-primary-foreground transition-transform focus:translate-y-0"
      >
        跳到主要内容
      </a>
      <AppSidebar
        collapsed={sidebarCollapsed}
        mobileOpen={mobileSidebarOpen}
        onCollapsedChange={handleCollapsedChange}
        onMobileOpenChange={setMobileSidebarOpen}
      />
      <div
        className={cn(
          "min-h-dvh min-w-0 overflow-x-hidden transition-[padding] duration-200",
          sidebarCollapsed ? "lg:pl-17" : "lg:pl-64",
        )}
      >
        <MonitorHeader onOpenNavigation={() => setMobileSidebarOpen(true)} />
        <main
          id="main-content"
          tabIndex={-1}
          className="mx-auto min-w-0 max-w-[100rem] space-y-4 px-3 py-4 pb-10 sm:px-5 sm:py-5 lg:space-y-5 lg:px-7 lg:py-6"
        >
          <Outlet />
        </main>
      </div>
    </div>
  )
}
