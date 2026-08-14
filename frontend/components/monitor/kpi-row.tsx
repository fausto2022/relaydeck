"use client"

import { DollarSign, HandCoins, Wallet } from "lucide-react"
import { Card } from "@/components/ui/card"
import { cn } from "@/lib/utils"
import { useDashboardSummary } from "@/lib/queries"
import { money } from "@/lib/format"
import type { LucideIcon } from "lucide-react"
import type { ReactNode } from "react"

interface Kpi {
  label: string
  value: ReactNode
  icon: LucideIcon
  iconBg: string
  iconColor: string
  footer: ReactNode
}

export function KpiRow() {
  const summary = useDashboardSummary()

  const data = summary.data
  const totalBalance = data?.total_balance ?? 0
  const todayTotalCost = data?.today_total_cost ?? 0
  const lowest = data?.lowest_balance ?? null
  const profit = data?.profit ?? null

  const kpis: Kpi[] = [
    {
      label: "总余额",
      value: money(totalBalance),
      icon: DollarSign,
      iconBg: "bg-brand/10",
      iconColor: "text-brand",
      footer: lowest ? (
        <span className="text-muted-foreground">
          {"最低："}
          <span className="font-medium text-foreground">{lowest.name}</span>
          {" "}
          <span className="text-warning">{money(lowest.balance)}</span>
        </span>
      ) : (
        <span className="text-muted-foreground">{"—"}</span>
      ),
    },
    {
      label: "今日消费",
      value: money(todayTotalCost),
      icon: Wallet,
      iconBg: "bg-warning/10",
      iconColor: "text-warning",
      footer: (
        <span className="text-muted-foreground">
          {todayTotalCost > 0 ? "已按渠道充值倍率换算" : "今日暂无消费"}
        </span>
      ),
    },
    {
      label: "今日利润",
      value: profit?.today_available ? (
        <span className={cn(profit.today_profit < 0 ? "text-danger" : "text-success")}>{money(profit.today_profit)}</span>
      ) : "—",
      icon: HandCoins,
      iconBg: "bg-success/10",
      iconColor: "text-success",
      footer: profit?.today_available ? (
        <span className="flex flex-col gap-0.5 text-muted-foreground">
          <span>收入 {money(profit.today_revenue)} · 成本 {money(profit.today_cost)}</span>
          <span>保底收入 {money(profit.today_guaranteed_revenue)} · 按 {(profit.guaranteed_revenue_ratio_basis_points / 100).toFixed(2)}% 折算</span>
        </span>
      ) : (
        <span className="text-muted-foreground">等待主站同步采样</span>
      ),
    },
  ]

  return (
    <Card className="dashboard-panel grid gap-0 overflow-hidden py-0 sm:grid-cols-3">
      {kpis.map((k) => (
        <div
          key={k.label}
          className="flex min-h-22 min-w-0 items-center gap-3 border-b border-border/70 px-4 py-3 last:border-b-0 sm:min-h-28 sm:border-b-0 sm:border-r sm:px-5 sm:last:border-r-0"
        >
          <span className={cn("flex size-10 shrink-0 items-center justify-center rounded-md ring-1 ring-inset ring-current/10", k.iconBg, k.iconColor)}>
            <k.icon className="size-5" />
          </span>
          <div className="flex min-w-0 flex-1 flex-col">
            <span className="text-xs font-medium text-muted-foreground">{k.label}</span>
            <p className="mt-0.5 text-xl font-semibold tabular-nums text-foreground sm:text-2xl">{k.value}</p>
            <p className="mt-1 min-w-0 text-[11px] leading-4 sm:text-xs">{k.footer}</p>
          </div>
        </div>
      ))}
    </Card>
  )
}
