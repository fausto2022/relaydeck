"use client"

import { Outlet } from "react-router-dom"
import { MonitorHeader } from "@/components/monitor/monitor-header"
import { DockBar } from "@/components/monitor/dock-bar"

/**
 * AppShell 是所有路由共享的外壳：顶部 header + 中间 Outlet（+ 可选底部 dock）。
 *
 * 当前 Dock 暂时隐藏 —— 单用户 / 少量数据下单页布局比拆页好。
 * 把 SHOW_DOCK 改成 true 即可恢复底部导航 + 路由跳转。
 */
const SHOW_DOCK = false

export function AppShell() {
  return (
    <div className="min-h-dvh min-w-0 bg-background">
      <a
        href="#main-content"
        className="fixed left-3 top-3 z-50 -translate-y-20 rounded-md bg-primary px-3 py-2 text-sm font-medium text-primary-foreground transition-transform focus:translate-y-0"
      >
        跳到主要内容
      </a>
      <MonitorHeader />
      <main
        id="main-content"
        tabIndex={-1}
        className={
          SHOW_DOCK
            ? "mx-auto max-w-360 space-y-4 px-3 py-4 pb-24 sm:px-5 sm:py-5 lg:space-y-5 lg:px-6"
            : "mx-auto max-w-360 space-y-4 px-3 py-4 sm:px-5 sm:py-5 lg:space-y-5 lg:px-6"
        }
      >
        <Outlet />
      </main>
      {SHOW_DOCK ? <DockBar /> : null}
    </div>
  )
}
