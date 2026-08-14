"use client"

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { useDashboardSummary } from "@/lib/queries"
import { money } from "@/lib/format"
import { cn } from "@/lib/utils"
import type { DashboardMainStationGroup } from "@/lib/api-types"

export function BalanceOverview() {
  const summary = useDashboardSummary()
  const channels = [...(summary.data?.channels ?? [])].sort(
    (a, b) => (b.today_cost ?? 0) - (a.today_cost ?? 0) || a.name.localeCompare(b.name),
  )
  const groups = [...(summary.data?.main_station_groups ?? [])].sort(
    (a, b) => groupCost(b) - groupCost(a)
      || a.group_name.localeCompare(b.group_name),
  )
  const unassignedGroups = groups.filter((group) => (group.channel_ids?.length ?? 0) === 0)
  const isLoading = summary.loading && !summary.data

  return (
    <Card className="border-border/80 lg:h-100">
      <CardHeader className="flex shrink-0 flex-row items-start justify-between gap-3 border-b border-border/70 px-4 pb-3 sm:px-5">
        <div className="min-w-0">
          <CardTitle className="text-base font-semibold">今日消耗排行</CardTitle>
          <p className="mt-1 text-xs text-muted-foreground">按账号今日消耗从高到低排列</p>
        </div>
        <span className="shrink-0 rounded-md bg-muted px-2 py-1 text-xs font-medium tabular-nums text-muted-foreground">
          {channels.length} 个账号
        </span>
      </CardHeader>
      <CardContent className="flex min-h-0 flex-1 flex-col px-4 pt-3 sm:px-5">
        {isLoading ? <div role="status" className="flex min-h-64 flex-1 items-center justify-center text-xs text-muted-foreground">加载中...</div> : null}
        {!isLoading && channels.length === 0 ? (
          <div className="flex min-h-64 flex-1 items-center justify-center text-xs text-muted-foreground">暂无账号消费数据</div>
        ) : null}
        {!isLoading && channels.length > 0 ? (
          <div className="min-h-0 flex-1 overflow-y-auto">
            <div className="divide-y divide-border/70">
              {channels.map((channel, index) => {
                const cost = channel.today_cost ?? 0
                const isFailed = !!channel.last_error
                const allLinkedGroups = groups.filter((group) => group.channel_ids?.includes(channel.id))
                const linkedGroups = allLinkedGroups.slice(0, 3)
                return (
                  <div key={channel.id} className="grid grid-cols-[2rem_minmax(0,1fr)_5.5rem] items-start gap-2.5 py-3 first:pt-0 sm:grid-cols-[2.25rem_minmax(0,1fr)_7rem_7rem] sm:gap-3">
                    <span
                      aria-label={`第 ${index + 1} 名`}
                      className={cn(
                        "flex size-7 items-center justify-center rounded-md text-xs font-semibold tabular-nums",
                        index === 0
                          ? "bg-primary text-primary-foreground"
                          : index < 3
                            ? "bg-primary/10 text-primary"
                            : "bg-muted text-muted-foreground",
                      )}
                    >
                      {index + 1}
                    </span>
                    <div className="min-w-0">
                      <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
                        <span className="break-words text-sm font-semibold text-foreground" title={channel.name}>{channel.name}</span>
                        <span className={cn("inline-flex items-center gap-1 text-xs", isFailed ? "text-danger" : "text-muted-foreground")}>
                          <span className={cn("size-1.5 rounded-full", isFailed ? "bg-danger" : channel.monitor_enabled ? "bg-success" : "bg-muted-foreground/50")} />
                          {isFailed ? "采集异常" : channel.monitor_enabled ? "监控中" : "未监控"}
                        </span>
                      </div>
                      <p className="mt-1 text-xs text-muted-foreground sm:hidden">
                        剩余余额 <span className="font-semibold tabular-nums text-foreground">{money(channel.last_balance)}</span>
                      </p>
                      <div className="mt-2 space-y-1">
                        {linkedGroups.length > 0 ? linkedGroups.map((group) => (
                          <GroupUsageLine key={group.group_id} group={group} />
                        )) : (
                          <p className="text-xs text-muted-foreground">暂无关联主站分组消耗</p>
                        )}
                        {allLinkedGroups.length > linkedGroups.length ? (
                          <p className="text-xs text-muted-foreground">另有 {allLinkedGroups.length - linkedGroups.length} 个关联分组</p>
                        ) : null}
                      </div>
                    </div>
                    <div className="text-right">
                      <span className="block text-base font-semibold tabular-nums text-foreground">{money(cost)}</span>
                      <span className="block text-xs text-muted-foreground">今日消耗</span>
                    </div>
                    <div className="hidden text-right sm:block">
                      <span className="block text-sm font-semibold tabular-nums text-foreground">{money(channel.last_balance)}</span>
                      <span className="block text-xs text-muted-foreground">剩余余额</span>
                    </div>
                  </div>
                )
              })}
            </div>
            {unassignedGroups.length > 0 ? <UnassignedGroups groups={unassignedGroups} /> : null}
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}

function GroupUsageLine({ group }: { group: DashboardMainStationGroup }) {
  const shared = (group.channel_ids?.length ?? 0) > 1
  const name = group.group_name || `分组 ${group.group_id}`
  return (
    <div className="flex min-w-0 items-center gap-1.5 rounded-md bg-muted/45 px-2 py-1 text-xs">
      <span className="truncate text-muted-foreground" title={name}>{name}</span>
      {shared ? <span className="shrink-0 text-primary">共享</span> : null}
      <span className="ml-auto shrink-0 font-semibold tabular-nums text-foreground">{money(groupCost(group))}</span>
    </div>
  )
}

function UnassignedGroups({ groups }: { groups: DashboardMainStationGroup[] }) {
  const visibleGroups = groups.slice(0, 4)
  return (
    <div className="border-t border-border/70 py-3">
      <div className="mb-2 flex items-center justify-between gap-2">
        <p className="text-xs font-medium text-muted-foreground">未映射到账号的主站分组</p>
        <span className="text-xs tabular-nums text-muted-foreground">{groups.length} 个</span>
      </div>
      <div className="flex flex-wrap gap-1.5">
        {visibleGroups.map((group) => (
          <span key={group.group_id} className="inline-flex max-w-full items-center gap-1.5 rounded-md bg-muted/45 px-2 py-1 text-xs">
            <span className="max-w-40 truncate text-muted-foreground">{group.group_name || `分组 ${group.group_id}`}</span>
            <span className="font-semibold tabular-nums text-foreground">{money(groupCost(group))}</span>
          </span>
        ))}
        {groups.length > visibleGroups.length ? (
          <span className="px-1 py-1 text-xs text-muted-foreground">另有 {groups.length - visibleGroups.length} 个</span>
        ) : null}
      </div>
    </div>
  )
}

function groupCost(group: DashboardMainStationGroup) {
  return group.actual_cost ?? group.account_cost ?? 0
}
