import { useEffect, useState, type ComponentType } from "react"
import { useTheme } from "next-themes"
import {
  Activity,
  BellRing,
  Captions,
  ChevronLeft,
  ChevronRight,
  ExternalLink,
  LayoutDashboard,
  ListOrdered,
  LogOut,
  Moon,
  RadioTower,
  ServerCog,
  Settings2,
  Sun,
  Tags,
  X,
} from "lucide-react"
import { Link, useLocation } from "react-router-dom"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { apiFetch } from "@/lib/api"
import type { AppVersion } from "@/lib/api-types"
import { useAuth } from "@/lib/auth-context"
import { useAppVersion } from "@/lib/queries"
import { cn } from "@/lib/utils"

interface NavigationItem {
  to: string
  label: string
  icon: ComponentType<{ className?: string }>
  tab?: string
}

const navigationGroups: Array<{ label: string; items: NavigationItem[] }> = [
  {
    label: "工作台",
    items: [
      { to: "/", label: "首页看板", icon: LayoutDashboard },
      { to: "/channels", label: "渠道管理", icon: RadioTower },
    ],
  },
  {
    label: "业务运营",
    items: [
      { to: "/main-station", label: "主站管理", icon: ServerCog },
      { to: "/rates", label: "倍率排行", icon: ListOrdered },
    ],
  },
  {
    label: "系统",
    items: [
      { to: "/settings", label: "系统设置", icon: Settings2, tab: "system" },
      { to: "/settings?tab=notifications", label: "通知渠道", icon: BellRing, tab: "notifications" },
      { to: "/settings?tab=captcha", label: "验证码服务", icon: Captions, tab: "captcha" },
      { to: "/settings?tab=rate-ranking", label: "倍率分类", icon: Tags, tab: "rate-ranking" },
    ],
  },
]

interface AppSidebarProps {
  collapsed: boolean
  mobileOpen: boolean
  onCollapsedChange: (collapsed: boolean) => void
  onMobileOpenChange: (open: boolean) => void
}

export function AppSidebar({
  collapsed,
  mobileOpen,
  onCollapsedChange,
  onMobileOpenChange,
}: AppSidebarProps) {
  return (
    <>
      <aside
        className={cn(
          "fixed inset-y-0 left-0 z-30 hidden flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground transition-[width] duration-200 lg:flex",
          collapsed ? "w-17" : "w-58",
        )}
      >
        <SidebarContent
          collapsed={collapsed}
          onNavigate={() => undefined}
          onCollapsedChange={onCollapsedChange}
        />
      </aside>

      <Dialog open={mobileOpen} onOpenChange={onMobileOpenChange}>
        <DialogContent
          showCloseButton={false}
          className="!top-0 !left-0 !flex h-dvh !max-h-dvh w-[min(18rem,calc(100vw-2rem))] !max-w-none !translate-x-0 !translate-y-0 flex-col gap-0 overflow-hidden rounded-none border-y-0 border-l-0 p-0 shadow-2xl lg:hidden"
        >
          <DialogTitle className="sr-only">主导航</DialogTitle>
          <Button
            variant="ghost"
            size="icon"
            className="absolute right-2 top-2 z-10"
            onClick={() => onMobileOpenChange(false)}
            aria-label="关闭主导航"
          >
            <X />
          </Button>
          <SidebarContent
            collapsed={false}
            onNavigate={() => onMobileOpenChange(false)}
          />
        </DialogContent>
      </Dialog>
    </>
  )
}

function SidebarContent({
  collapsed,
  onNavigate,
  onCollapsedChange,
}: {
  collapsed: boolean
  onNavigate: () => void
  onCollapsedChange?: (collapsed: boolean) => void
}) {
  const location = useLocation()
  const { resolvedTheme, setTheme } = useTheme()
  const { username, authDisabled, logout } = useAuth()
  const appVersion = useAppVersion()
  const [mounted, setMounted] = useState(false)
  const [checkingVersion, setCheckingVersion] = useState(false)
  const appTitle = appVersion.data?.title?.trim() || "RelayDeck"
  const version = appVersion.data?.version?.trim()
  const latestVersion = appVersion.data?.latest_version?.trim()
  const updateAvailable = Boolean(appVersion.data?.update_available && latestVersion)
  const updateURL = appVersion.data?.release_url?.trim() || appVersion.data?.repo_url?.trim()

  useEffect(() => setMounted(true), [])

  useEffect(() => {
    document.title = appTitle
  }, [appTitle])

  async function handleCheckVersion() {
    setCheckingVersion(true)
    try {
      const result = await apiFetch<AppVersion>("/version?force=1")
      appVersion.setData(result)
      if (result.update_error) toast.error(result.update_error)
      else if (result.update_available && result.latest_version) toast.warning(`发现新版本 ${result.latest_version}`)
      else toast.success("当前已是最新版本")
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "检测更新失败")
    } finally {
      setCheckingVersion(false)
    }
  }

  const activeTab = new URLSearchParams(location.search).get("tab") || "system"
  const darkMode = mounted && resolvedTheme === "dark"

  return (
    <div className="flex h-full min-h-0 flex-1 flex-col">
      <div className={cn("flex h-16 shrink-0 items-center border-b border-sidebar-border", collapsed ? "justify-center px-2" : "gap-3 px-4")}>
        <span className="flex size-9 shrink-0 items-center justify-center rounded-md bg-sidebar-primary text-sidebar-primary-foreground shadow-sm">
          <Activity className="size-4.5" strokeWidth={2.5} />
        </span>
        {!collapsed ? (
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-semibold">{appTitle}</div>
            <button
              type="button"
              onClick={() => void handleCheckVersion()}
              disabled={checkingVersion}
              className="mt-0.5 flex max-w-full items-center gap-1.5 truncate text-xs text-muted-foreground hover:text-sidebar-foreground disabled:opacity-60"
            >
              {checkingVersion ? "检测更新中" : version ? `v${version}` : "检测版本"}
              {updateAvailable ? <span className="size-1.5 shrink-0 rounded-full bg-success" aria-label="有新版本" /> : null}
            </button>
          </div>
        ) : null}
      </div>

      <nav aria-label="主导航" className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-2 py-3">
        {navigationGroups.map((group) => (
          <div key={group.label} className="mb-4 last:mb-0">
            {!collapsed ? <div className="mb-1 px-2 text-[11px] font-medium text-muted-foreground">{group.label}</div> : null}
            <div className="space-y-1">
              {group.items.map((item) => {
                const active = item.tab
                  ? location.pathname === "/settings" && activeTab === item.tab
                  : location.pathname === item.to
                const Icon = item.icon
                const link = (
                  <Link
                    key={item.to}
                    to={item.to}
                    onClick={onNavigate}
                    aria-current={active ? "page" : undefined}
                    className={cn(
                      "relative flex min-h-10 items-center rounded-md text-sm font-medium outline-none transition-[background-color,color,box-shadow] focus-visible:ring-[3px] focus-visible:ring-sidebar-ring/35",
                      collapsed ? "justify-center px-2" : "gap-3 px-3",
                      active
                        ? "bg-sidebar-accent text-sidebar-accent-foreground shadow-sm"
                        : "text-muted-foreground hover:bg-sidebar-accent/60 hover:text-sidebar-foreground",
                    )}
                  >
                    {active ? <span className="absolute inset-y-2 left-0 w-0.5 rounded-full bg-sidebar-primary" /> : null}
                    <Icon className={cn("size-4.5 shrink-0", active && "text-sidebar-primary")} />
                    {!collapsed ? <span className="truncate">{item.label}</span> : <span className="sr-only">{item.label}</span>}
                  </Link>
                )

                if (!collapsed) return link
                return (
                  <Tooltip key={`${item.to}-${item.tab || ""}`} delayDuration={150}>
                    <TooltipTrigger asChild>{link}</TooltipTrigger>
                    <TooltipContent side="right">{item.label}</TooltipContent>
                  </Tooltip>
                )
              })}
            </div>
          </div>
        ))}
      </nav>

      <div className="shrink-0 border-t border-sidebar-border p-2">
        {updateAvailable && !collapsed ? (
          <a
            href={updateURL || "https://github.com/fausto2022/relaydeck"}
            target="_blank"
            rel="noopener noreferrer"
            className="mb-2 flex min-h-10 items-center gap-2 rounded-md bg-success/10 px-3 text-xs font-medium text-success hover:bg-success/15"
          >
            <ExternalLink className="size-4" />
            <span className="truncate">发现新版本 {latestVersion}</span>
          </a>
        ) : null}
        <SidebarAction
          collapsed={collapsed}
          label={darkMode ? "切换浅色模式" : "切换深色模式"}
          icon={darkMode ? Moon : Sun}
          onClick={() => setTheme(darkMode ? "light" : "dark")}
        />
        {authDisabled ? null : (
          <SidebarAction
            collapsed={collapsed}
            label={username ? `${username} · 退出登录` : "退出登录"}
            icon={LogOut}
            onClick={logout}
            danger
          />
        )}
        {onCollapsedChange ? (
          <SidebarAction
            collapsed={collapsed}
            label={collapsed ? "展开侧栏" : "收起侧栏"}
            icon={collapsed ? ChevronRight : ChevronLeft}
            onClick={() => onCollapsedChange(!collapsed)}
          />
        ) : null}
      </div>
    </div>
  )
}

function SidebarAction({
  collapsed,
  label,
  icon: Icon,
  onClick,
  danger = false,
}: {
  collapsed: boolean
  label: string
  icon: ComponentType<{ className?: string }>
  onClick: () => void
  danger?: boolean
}) {
  const button = (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "flex min-h-10 w-full items-center rounded-md text-sm font-medium outline-none transition-colors focus-visible:ring-[3px] focus-visible:ring-sidebar-ring/35",
        collapsed ? "justify-center px-2" : "gap-3 px-3",
        danger
          ? "text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
          : "text-muted-foreground hover:bg-sidebar-accent/60 hover:text-sidebar-foreground",
      )}
    >
      <Icon className="size-4.5 shrink-0" />
      {!collapsed ? <span className="truncate">{label}</span> : <span className="sr-only">{label}</span>}
    </button>
  )

  if (!collapsed) return button
  return (
    <Tooltip delayDuration={150}>
      <TooltipTrigger asChild>{button}</TooltipTrigger>
      <TooltipContent side="right">{label}</TooltipContent>
    </Tooltip>
  )
}
