"use client"

import { ListOrdered } from "lucide-react"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { useDashboardSummary } from "@/lib/queries"
import { money } from "@/lib/format"
import { cn } from "@/lib/utils"

export function BalanceOverview() {
  const summary = useDashboardSummary()
  const channels = [...(summary.data?.channels ?? [])].sort(
    (a, b) => (b.today_cost ?? 0) - (a.today_cost ?? 0) || a.name.localeCompare(b.name),
  )
  const isLoading = summary.loading && !summary.data

  return (
    <Card className="dashboard-panel min-h-72 gap-0 overflow-hidden border-border/80 py-0 lg:h-96">
      <CardHeader className="dashboard-panel-header flex shrink-0 flex-row items-center justify-between gap-3 px-4 py-3 sm:px-5">
        <div className="flex min-w-0 items-center gap-2.5">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
            <ListOrdered className="size-4" />
          </span>
          <CardTitle className="truncate text-sm font-semibold sm:text-base">今日消耗排行</CardTitle>
        </div>
        <span className="shrink-0 rounded-md bg-card px-2 py-1 text-xs font-medium tabular-nums text-muted-foreground ring-1 ring-border/80">
          {channels.length} 个账号
        </span>
      </CardHeader>
      <CardContent className="flex min-h-0 flex-1 flex-col px-4 sm:px-5">
        {isLoading ? <div role="status" className="flex min-h-44 flex-1 items-center justify-center text-xs text-muted-foreground">加载中...</div> : null}
        {!isLoading && channels.length === 0 ? (
          <div className="flex min-h-44 flex-1 flex-col items-center justify-center gap-2 text-xs text-muted-foreground">
            <span className="flex size-10 items-center justify-center rounded-md bg-muted text-muted-foreground">
              <ListOrdered className="size-4" />
            </span>
            暂无账号消费数据
          </div>
        ) : null}
        {!isLoading && channels.length > 0 ? (
          <div className="min-h-0 flex-1 overflow-y-auto">
            <div className="sticky top-0 z-10 grid grid-cols-[minmax(4.5rem,1fr)_5.5rem_5.5rem] gap-2 border-b border-border/70 bg-card/95 py-2 text-[11px] font-medium text-muted-foreground backdrop-blur-sm sm:grid-cols-[minmax(0,1fr)_7rem_7rem] sm:gap-3 sm:text-xs">
              <span>账号</span>
              <span className="text-right">今日消耗</span>
              <span className="text-right">余额</span>
            </div>
            <div className="divide-y divide-border/70">
              {channels.map((channel, index) => {
                const cost = channel.today_cost ?? 0
                return (
                  <div key={channel.id} className="grid grid-cols-[minmax(4.5rem,1fr)_5.5rem_5.5rem] items-center gap-2 py-2.5 transition-colors hover:bg-muted/30 sm:grid-cols-[minmax(0,1fr)_7rem_7rem] sm:gap-3">
                    <div className="flex min-w-0 items-center gap-2">
                      <span
                        aria-label={`第 ${index + 1} 名`}
                        className={cn(
                          "flex size-6 shrink-0 items-center justify-center rounded-md text-[11px] font-semibold tabular-nums",
                          index === 0
                            ? "bg-foreground text-background"
                            : index < 3
                              ? "bg-primary/10 text-primary"
                              : "bg-muted text-muted-foreground",
                        )}
                      >
                        {index + 1}
                      </span>
                      <span className="truncate text-sm font-medium text-foreground" title={channel.name}>{channel.name}</span>
                    </div>
                    <span className="truncate text-right font-mono text-[11px] font-semibold tabular-nums text-primary sm:text-sm" title={money(cost)}>{money(cost)}</span>
                    <span className="truncate text-right font-mono text-[11px] tabular-nums text-foreground sm:text-sm" title={money(channel.last_balance)}>{money(channel.last_balance)}</span>
                  </div>
                )
              })}
            </div>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
