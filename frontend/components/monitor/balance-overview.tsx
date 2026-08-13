"use client"

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { useDashboardSummary } from "@/lib/queries"
import { money } from "@/lib/format"
import { cn } from "@/lib/utils"

export function BalanceOverview() {
  const summary = useDashboardSummary()
  const channels = [...(summary.data?.channels ?? [])].sort(
    (a, b) => (b.today_cost ?? 0) - (a.today_cost ?? 0) || a.name.localeCompare(b.name),
  )
  const groups = [...(summary.data?.main_station_groups ?? [])].sort(
    (a, b) => (b.actual_cost ?? b.account_cost ?? 0) - (a.actual_cost ?? a.account_cost ?? 0)
      || a.group_name.localeCompare(b.group_name),
  )
  const maxCost = Math.max(...channels.map((channel) => channel.today_cost ?? 0), 0)
  const hasCost = maxCost > 0
  const isLoading = summary.loading && !summary.data

  return (
    <Card className="border border-border shadow-none lg:h-100">
      <CardHeader className="flex shrink-0 flex-row items-center justify-between px-4 pb-2 sm:px-6">
        <CardTitle className="text-base font-semibold">{"今日消耗排行"}</CardTitle>
        <span className="text-xs text-muted-foreground">{"渠道与主站分组"}</span>
      </CardHeader>
      <CardContent className="flex min-h-0 flex-1 flex-col px-4 sm:px-6">
        {isLoading ? <div className="flex min-h-64 flex-1 items-center justify-center text-xs text-muted-foreground">{"加载中…"}</div> : null}
        {!isLoading && channels.length === 0 ? (
          <div className="flex min-h-64 flex-1 items-center justify-center text-xs text-muted-foreground">{"暂无渠道消费数据"}</div>
        ) : null}
        {!isLoading && channels.length > 0 ? (
          <div className="min-h-0 flex-1 overflow-y-auto">
            <div className="space-y-2">
              {channels.map((channel, index) => {
                const cost = channel.today_cost ?? 0
                const isFailed = !!channel.last_error
                const width = hasCost ? Math.max((cost / maxCost) * 100, cost > 0 ? 3 : 0) : 0
                return (
                  <div key={channel.id} className="grid grid-cols-[1.5rem_minmax(0,1fr)_4.75rem_4.75rem] items-center gap-1.5 rounded-md border border-border/70 px-2 py-2 sm:grid-cols-[2rem_minmax(0,1fr)_7rem_7rem] sm:gap-3 sm:px-3">
                    <span className={cn("text-center text-xs font-semibold tabular-nums", index < 3 ? "text-brand" : "text-muted-foreground")}>#{index + 1}</span>
                    <div className="min-w-0">
                      <div className="flex min-w-0 items-center gap-1.5">
                        <span className={cn("size-2 shrink-0 rounded-full", isFailed ? "bg-danger" : cost > 0 ? "bg-warning" : "bg-muted-foreground/40")} />
                        <span className="truncate text-sm font-medium text-foreground">{channel.name}</span>
                      </div>
                      <div className="mt-1 h-1.5 w-full overflow-hidden rounded-full bg-muted">
                        <div className="h-full rounded-full bg-warning transition-[width]" style={{ width: `${width}%` }} />
                      </div>
                    </div>
                    <div className="text-right">
                      <span className="block text-xs font-semibold tabular-nums text-foreground sm:text-sm">{money(cost)}</span>
                      <span className="block text-[10px] text-muted-foreground">今日消耗</span>
                    </div>
                    <div className="text-right">
                      <span className="block text-xs tabular-nums text-muted-foreground sm:text-sm">{money(channel.last_balance)}</span>
                      <span className="block text-[10px] text-muted-foreground">当前余额</span>
                    </div>
                  </div>
                )
              })}
            </div>
            <div className="mt-4 border-t border-border/70 pt-3">
              <div className="mb-2 flex items-center justify-between gap-2">
                <p className="text-sm font-semibold text-foreground">{"主站分组今日消耗"}</p>
                <span className="text-[10px] text-muted-foreground">{"按实际消耗降序"}</span>
              </div>
              {groups.length === 0 ? (
                <p className="py-3 text-center text-xs text-muted-foreground">{"暂无主站分组消费数据"}</p>
              ) : (
                <div className="space-y-1.5">
                  {groups.map((group, index) => {
                    const cost = group.actual_cost ?? group.account_cost
                    return (
                      <div key={`${group.group_id}-${group.group_name}`} className="grid grid-cols-[1.5rem_minmax(0,1fr)_5.5rem] items-center gap-2 rounded-md bg-muted/35 px-2.5 py-2 sm:grid-cols-[2rem_minmax(0,1fr)_7rem] sm:gap-3 sm:px-3">
                        <span className="text-center text-xs font-semibold tabular-nums text-muted-foreground">#{index + 1}</span>
                        <div className="min-w-0">
                          <p className="truncate text-sm font-medium text-foreground">{group.group_name || `分组 ${group.group_id}`}</p>
                          <p className="truncate text-[10px] text-muted-foreground">{group.requests.toLocaleString("zh-CN")} 次 · {group.total_tokens.toLocaleString("zh-CN")} Token</p>
                        </div>
                        <div className="text-right">
                          <span className="text-sm font-semibold tabular-nums text-foreground">{money(cost)}</span>
                          <span className="block text-[10px] text-muted-foreground">今日消耗</span>
                        </div>
                      </div>
                    )
                  })}
                </div>
              )}
            </div>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
