"use client"

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { useDashboardSummary } from "@/lib/queries"
import { money } from "@/lib/format"

export function BalanceOverview() {
  const summary = useDashboardSummary()
  const channels = [...(summary.data?.channels ?? [])].sort(
    (a, b) => (b.today_cost ?? 0) - (a.today_cost ?? 0) || a.name.localeCompare(b.name),
  )
  const isLoading = summary.loading && !summary.data

  return (
    <Card className="gap-0 border-border/80 lg:h-100">
      <CardHeader className="flex shrink-0 flex-row items-center justify-between gap-3 border-b border-border/70 px-4 pb-3 sm:px-5">
        <CardTitle className="text-base font-semibold">今日消耗排行</CardTitle>
        <span className="shrink-0 rounded-md bg-muted px-2 py-1 text-xs font-medium tabular-nums text-muted-foreground">
          {channels.length} 个账号
        </span>
      </CardHeader>
      <CardContent className="flex min-h-0 flex-1 flex-col px-4 sm:px-5">
        {isLoading ? <div role="status" className="flex min-h-64 flex-1 items-center justify-center text-xs text-muted-foreground">加载中...</div> : null}
        {!isLoading && channels.length === 0 ? (
          <div className="flex min-h-64 flex-1 items-center justify-center text-xs text-muted-foreground">暂无账号消费数据</div>
        ) : null}
        {!isLoading && channels.length > 0 ? (
          <div className="min-h-0 flex-1 overflow-y-auto">
            <div className="sticky top-0 z-10 grid grid-cols-[minmax(0,1fr)_5rem_5rem] gap-2 border-b border-border/70 bg-card py-2 text-xs font-medium text-muted-foreground sm:grid-cols-[minmax(0,1fr)_7rem_7rem] sm:gap-3">
              <span>账号</span>
              <span className="text-right">今日消耗</span>
              <span className="text-right">余额</span>
            </div>
            <div className="divide-y divide-border/70">
              {channels.map((channel, index) => {
                const cost = channel.today_cost ?? 0
                return (
                  <div key={channel.id} className="grid grid-cols-[minmax(0,1fr)_5rem_5rem] items-center gap-2 py-2.5 sm:grid-cols-[minmax(0,1fr)_7rem_7rem] sm:gap-3">
                    <div className="flex min-w-0 items-center gap-1.5">
                      <span aria-label={`第 ${index + 1} 名`} className="w-6 shrink-0 text-xs font-semibold tabular-nums text-muted-foreground">
                        {index + 1}
                      </span>
                      <span className="truncate text-sm font-medium text-foreground" title={channel.name}>{channel.name}</span>
                    </div>
                    <span className="text-right text-xs font-semibold tabular-nums text-foreground sm:text-sm">{money(cost)}</span>
                    <span className="text-right text-xs tabular-nums text-foreground sm:text-sm">{money(channel.last_balance)}</span>
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
