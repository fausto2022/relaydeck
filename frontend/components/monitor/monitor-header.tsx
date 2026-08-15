import { useMemo, useState } from "react"
import { Menu, RefreshCw } from "lucide-react"
import { useLocation } from "react-router-dom"
import { Button } from "@/components/ui/button"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import { useTriggerRefresh } from "@/lib/refresh-context"
import { useChannels } from "@/lib/queries"
import { relativeTime } from "@/lib/format"

const pageMeta: Record<string, { section: string; title: string }> = {
  "/": { section: "工作台", title: "首页看板" },
  "/channels": { section: "工作台", title: "渠道管理" },
  "/main-station": { section: "业务运营", title: "主站管理" },
  "/rates": { section: "业务运营", title: "倍率排行" },
  "/settings": { section: "系统", title: "系统设置" },
}

const settingsTitle: Record<string, string> = {
  system: "系统设置",
  notifications: "通知渠道",
  captcha: "验证码服务",
  "rate-ranking": "倍率分类",
}

export function MonitorHeader({ onOpenNavigation }: { onOpenNavigation: () => void }) {
  const location = useLocation()
  const refresh = useTriggerRefresh()
  const channels = useChannels()
  const [syncing, setSyncing] = useState(false)

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

  const meta = pageMeta[location.pathname] ?? { section: "工作台", title: "管理后台" }
  const activeSettingsTab = new URLSearchParams(location.search).get("tab") || "system"
  const title = location.pathname === "/settings" ? settingsTitle[activeSettingsTab] || meta.title : meta.title

  return (
    <header className="sticky top-0 z-20 border-b border-border/80 bg-background/94 backdrop-blur-md">
      <div className="mx-auto flex h-14 max-w-400 items-center gap-2 px-3 sm:px-5 lg:h-15 lg:px-6">
        <Button
          variant="ghost"
          size="icon"
          className="lg:hidden"
          onClick={onOpenNavigation}
          aria-label="打开主导航"
        >
          <Menu />
        </Button>
        <div className="min-w-0 flex-1">
          <div className="hidden text-[11px] font-medium text-muted-foreground sm:block">{meta.section}</div>
          <div className="truncate text-sm font-semibold text-foreground sm:text-base">
            {title}
          </div>
        </div>
        <div className="flex items-center justify-end gap-1.5">
          <span className="mr-1 hidden items-center gap-1.5 text-xs text-muted-foreground md:inline-flex">
            <span className="size-1.5 rounded-full bg-success" />
            上次采集 <span className="font-medium text-foreground">{relativeTime(lastCollectedAt)}</span>
          </span>
          <Tooltip delayDuration={200}>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="icon"
                onClick={handleRefresh}
                disabled={syncing}
                className="bg-card shadow-sm"
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
        </div>
      </div>
    </header>
  )
}
