import { useEffect, useMemo, useState } from "react"
import { useTheme } from "next-themes"
import { Activity, Home, LogOut, RefreshCw, ServerCog, Sun, Moon, Settings } from "lucide-react"
import { Link, useLocation } from "react-router-dom"
import { Button } from "@/components/ui/button"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import { useAuth } from "@/lib/auth-context"
import { apiFetch } from "@/lib/api"
import { useTriggerRefresh } from "@/lib/refresh-context"
import { useAppVersion, useChannels } from "@/lib/queries"
import type { AppVersion } from "@/lib/api-types"
import { relativeTime } from "@/lib/format"
import { toast } from "sonner"

export function MonitorHeader() {
  const location = useLocation()
  const { resolvedTheme, setTheme } = useTheme()
  const { username, authDisabled, logout } = useAuth()
  const refresh = useTriggerRefresh()
  const channels = useChannels()
  const appVersion = useAppVersion()
  const [mounted, setMounted] = useState(false)
  const [syncing, setSyncing] = useState(false)
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

  /**
   * 找出所有渠道中最近一次采集时间——这是"上次采集"展示的依据，
   * 让用户知道页面上的余额到底是多新的快照（区别于"我刚点了刷新"）。
   */
  const lastCollectedAt = useMemo(() => {
    const list = channels.data ?? []
    let best: string | null = null
    let bestT = -Infinity
    for (const c of list) {
      if (!c.last_balance_at) continue
      const t = new Date(c.last_balance_at).getTime()
      if (Number.isFinite(t) && t > bestT) {
        bestT = t
        best = c.last_balance_at
      }
    }
    return best
  }, [channels.data])

  function handleRefresh() {
    setSyncing(true)
    refresh()
    setTimeout(() => setSyncing(false), 800)
  }

  async function handleCheckVersion() {
    setCheckingVersion(true)
    try {
      const result = await apiFetch<AppVersion>("/version?force=1")
      appVersion.setData(result)
      if (result.update_error) {
        toast.error(result.update_error)
      } else if (result.update_available && result.latest_version) {
        toast.warning(`发现新版本 ${result.latest_version}`)
      } else {
        toast.success("当前已是最新版本")
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "检测更新失败")
    } finally {
      setCheckingVersion(false)
    }
  }

  const navItems = [
    { to: "/", label: "主页", icon: Home },
    { to: "/main-station", label: "主站", icon: ServerCog },
    { to: "/settings", label: "设置", icon: Settings },
  ]

  const navigation = (mobile: boolean) => navItems.map((item) => {
    const active = location.pathname === item.to
    const Icon = item.icon
    return (
      <Link
        key={item.to}
        to={item.to}
        aria-current={active ? "page" : undefined}
        className={cn(
          "inline-flex items-center justify-center gap-1.5 rounded-sm font-medium outline-none transition-colors focus-visible:ring-[3px] focus-visible:ring-ring/35",
          mobile ? "h-10 text-xs" : "h-8 px-3 text-sm",
          active
            ? "bg-card text-primary ring-1 ring-border/80"
            : "text-muted-foreground hover:bg-card/70 hover:text-foreground",
        )}
      >
        <Icon className="size-4" />
        <span>{item.label}</span>
      </Link>
    )
  })

  const darkMode = mounted && resolvedTheme === "dark"

  return (
    <header className="sticky top-0 z-20 border-b border-border/80 bg-card/92 backdrop-blur-md">
      <div className="mx-auto flex min-h-14 max-w-360 flex-wrap items-center gap-x-3 px-3 py-2 sm:px-5 md:h-16 md:flex-nowrap md:gap-4 md:py-0 lg:px-6">
        <div className="flex min-w-0 flex-1 items-center gap-2.5">
          <div className="flex size-9 shrink-0 items-center justify-center rounded-md bg-primary text-primary-foreground">
            <Activity className="size-4.5" strokeWidth={2.5} />
          </div>
          <div className="min-w-0">
            <h1 className="truncate text-base font-semibold text-foreground">{appTitle}</h1>
            {version ? (
              <p className="truncate text-xs leading-4 text-muted-foreground">
                <button
                  type="button"
                  className="font-medium underline-offset-2 hover:text-foreground hover:underline disabled:opacity-60"
                  onClick={handleCheckVersion}
                  disabled={checkingVersion}
                  title="点击检测更新"
                >
                  {checkingVersion ? "检测中..." : `v${version}`}
                </button>
                {updateAvailable ? (
                  <a
                    href={updateURL || "https://github.com/fausto2022/relaydeck"}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="ml-2 font-medium text-success underline-offset-2 hover:underline"
                  >
                    有新版本 {latestVersion}
                  </a>
                ) : null}
              </p>
            ) : null}
          </div>
        </div>

        <nav aria-label="主导航" className="hidden items-center gap-1 rounded-md border border-border/70 bg-muted/60 p-1 md:flex">
          {navigation(false)}
        </nav>

        <div className="flex flex-1 items-center justify-end gap-1.5">
          <span className="mr-1 hidden text-xs text-muted-foreground xl:inline">
            上次采集 <span className="font-medium text-foreground">{relativeTime(lastCollectedAt)}</span>
          </span>
          <Tooltip delayDuration={200}>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="icon"
                onClick={handleRefresh}
                disabled={syncing}
                className="size-10 md:size-9"
                aria-label="刷新视图"
              >
                <RefreshCw className={cn("size-4", syncing && "animate-spin")} />
              </Button>
            </TooltipTrigger>
            <TooltipContent side="bottom" className="max-w-xs text-xs">
              <p>重新拉取最新的快照数据。</p>
              <p className="mt-1 text-muted-foreground">实际采集由后台定时任务执行，如需立即采集请到具体渠道点“同步”。</p>
            </TooltipContent>
          </Tooltip>

          <Tooltip delayDuration={200}>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="icon"
                onClick={() => setTheme(darkMode ? "light" : "dark")}
                className="size-10 md:size-9"
                aria-label="切换主题"
              >
                {darkMode ? <Moon className="size-4" /> : <Sun className="size-4" />}
              </Button>
            </TooltipTrigger>
            <TooltipContent side="bottom" className="text-xs">
              {darkMode ? "深色模式 · 点击切换浅色" : "浅色模式 · 点击切换深色"}
            </TooltipContent>
          </Tooltip>

          {authDisabled ? null : (
            <Tooltip delayDuration={200}>
              <TooltipTrigger asChild>
                <Button
                  variant="outline"
                  size="icon"
                  onClick={logout}
                  className="size-10 md:size-9"
                  aria-label="退出登录"
                >
                  <LogOut className="size-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent side="bottom" className="text-xs">
                {username ? `${username} · 退出登录` : "退出登录"}
              </TooltipContent>
            </Tooltip>
          )}
        </div>

        <nav aria-label="移动端主导航" className="grid basis-full grid-cols-3 gap-1 rounded-md border border-border/70 bg-muted/60 p-1 md:hidden">
          {navigation(true)}
        </nav>
      </div>
    </header>
  )
}
