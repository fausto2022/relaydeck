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

const pageMeta: Record<string, { section: string; title: string; description: string }> = {
  "/": { section: "工作台", title: "运行总览", description: "余额、消耗与渠道状态" },
  "/channels": { section: "工作台", title: "渠道管理", description: "上游账号与同步作业" },
  "/main-station": { section: "业务运营", title: "主站管理", description: "账号池、健康探测与调度" },
  "/rates": { section: "业务运营", title: "倍率排行", description: "成本倍率与价格变化" },
  "/settings": { section: "系统", title: "系统设置", description: "运行策略与系统服务" },
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

  const meta = pageMeta[location.pathname] ?? { section: "工作台", title: "管理后台", description: "" }
  const activeSettingsTab = new URLSearchParams(location.search).get("tab") || "system"
  const title = location.pathname === "/settings" ? settingsTitle[activeSettingsTab] || meta.title : meta.title

  return (
    <header className="sticky top-0 z-20 border-b border-border/80 bg-card/92 backdrop-blur-xl">
      <div className="mx-auto flex min-h-16 max-w-[100rem] items-center gap-3 px-3 py-2 sm:px-5 lg:min-h-[4.5rem] lg:px-7">
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
          <div className="flex min-w-0 items-baseline gap-2">
            <h1 className="truncate text-base font-semibold text-foreground lg:text-lg">{title}</h1>
            <span className="hidden text-[11px] text-muted-foreground sm:inline">/ {meta.section}</span>
          </div>
          <p className="mt-0.5 hidden truncate text-xs text-muted-foreground sm:block">{meta.description}</p>
        </div>
        <div className="flex items-center justify-end gap-1.5">
          <span className="mr-1 hidden items-center gap-2 rounded-md border border-border bg-muted/35 px-2.5 py-1.5 text-xs text-muted-foreground md:inline-flex">
            <span className="relative flex size-2"><span className="absolute inline-flex size-full animate-ping rounded-full bg-success/40" /><span className="relative inline-flex size-2 rounded-full bg-success" /></span>
            采集状态 <span className="font-medium text-foreground">{relativeTime(lastCollectedAt)}</span>
          </span>
          <Tooltip delayDuration={200}>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="icon"
                onClick={handleRefresh}
                disabled={syncing}
                className="bg-card"
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
