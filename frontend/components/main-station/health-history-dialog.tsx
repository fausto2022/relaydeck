import { useCallback, useEffect, useMemo, useState } from "react"
import { Activity, RefreshCw } from "lucide-react"
import { toast } from "sonner"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Spinner } from "@/components/ui/spinner"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { apiFetch } from "@/lib/api"
import type {
  MainStationAccount,
  MainStationHealthCheck,
  MainStationHealthStats,
  MainStationMemberHealthSummary,
  MainStationPage,
} from "@/lib/api-types"
import { dateTime, relativeTime } from "@/lib/format"
import { cn } from "@/lib/utils"

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  groupID: number | null
  account: MainStationAccount | null
}

export function HealthHistoryDialog({ open, onOpenChange, groupID, account }: Props) {
  const [checks, setChecks] = useState<MainStationHealthCheck[]>([])
  const [stats, setStats] = useState<MainStationHealthStats | null>(null)
  const [loading, setLoading] = useState(false)
  const [refreshing, setRefreshing] = useState(false)

  const load = useCallback(async (silent = false) => {
    if (!groupID || !account?.member) return
    if (silent) setRefreshing(true)
    else setLoading(true)
    try {
      const [history, summary] = await Promise.all([
        apiFetch<MainStationPage<MainStationHealthCheck>>(
          `/main-station/groups/${groupID}/health-checks?member_id=${account.member.id}&page=1&page_size=60`,
        ),
        apiFetch<{ items: MainStationMemberHealthSummary[] }>(`/main-station/groups/${groupID}/health-summary`),
      ])
      setChecks(history.items)
      setStats(summary.items.find((item) => item.member.id === account.member?.id)?.stats ?? null)
    } catch (error) {
      if (!silent) toast.error(error instanceof Error ? error.message : "加载探测记录失败")
    } finally {
      if (silent) setRefreshing(false)
      else setLoading(false)
    }
  }, [account, groupID])

  useEffect(() => {
    if (!open || !account?.member || !groupID) return
    void load()
    const timer = window.setInterval(() => void load(true), 10_000)
    return () => window.clearInterval(timer)
  }, [account?.member?.id, groupID, load, open])

  const timeline = useMemo(() => [...checks].slice(0, 60).reverse(), [checks])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader className="pr-8">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <DialogTitle className="flex items-center gap-2 truncate">
                <Activity className="size-5 shrink-0 text-emerald-600" />
                <span className="truncate">{account?.name ?? "账号"} · 探测记录</span>
              </DialogTitle>
              <DialogDescription className="mt-1">{account?.member?.source_group_name || "默认来源分组"}</DialogDescription>
            </div>
            <Button variant="outline" size="icon" aria-label="刷新探测记录" onClick={() => void load(true)} disabled={loading || refreshing}>
              <RefreshCw className={cn("size-4", refreshing && "animate-spin")} />
            </Button>
          </div>
        </DialogHeader>

        {!account?.member ? <div className="py-8 text-center text-sm text-muted-foreground">账号尚未接管，暂无探测记录</div> : null}
        {account?.member && loading ? <div className="flex justify-center py-12"><Spinner /></div> : null}
        {account?.member && !loading ? (
          <div className="space-y-5">
            <div className="grid gap-3 sm:grid-cols-3">
              <Stat label="可用性 · 7 天" value={formatPercent(stats?.seven_day_success_rate)} tone={successTone(stats?.seven_day_success_rate)} />
              <Stat label="平均延迟" value={formatLatency(stats?.average_latency_ms)} />
              <Stat label="P95 延迟" value={formatLatency(stats?.p95_latency_ms)} />
            </div>

            <section className="space-y-2">
              <div className="flex items-center justify-between text-sm">
                <span className="font-medium">最近 60 次</span>
                <span className="text-xs text-muted-foreground">{stats?.last_success_at ? `最近成功 ${relativeTime(stats.last_success_at)}` : "暂无成功记录"}</span>
              </div>
              {timeline.length > 0 ? (
                <div className="grid h-10 grid-flow-col auto-cols-fr items-end gap-0.5 rounded border bg-muted/20 p-1" title="从左到右为较早到较新的探测记录">
                  {timeline.map((check) => (
                    <span
                      key={check.id}
                      className={cn("min-h-2 rounded-sm", check.status === "success" ? "bg-emerald-500" : check.status === "skipped_budget" ? "bg-amber-400" : "bg-red-500")}
                      style={{ height: `${Math.max(25, Math.min(100, check.latency_ms > 0 ? 25 + Math.log10(check.latency_ms + 1) * 22 : 30))}%` }}
                      title={`${dateTime(check.created_at)} · ${statusLabel(check.status)} · ${formatLatency(check.latency_ms)}`}
                    />
                  ))}
                </div>
              ) : <div className="rounded border border-dashed py-8 text-center text-sm text-muted-foreground">暂无探测记录</div>}
              <div className="flex justify-between text-[11px] uppercase tracking-wide text-muted-foreground"><span>较早</span><span>现在</span></div>
            </section>

            {stats?.last_error_message ? (
              <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-800">
                最近错误：{stats.last_error_message}
              </div>
            ) : null}

            <div className="overflow-x-auto border">
              <Table>
                <TableHeader><TableRow><TableHead>时间</TableHead><TableHead>级别</TableHead><TableHead>状态</TableHead><TableHead>延迟</TableHead><TableHead>HTTP</TableHead><TableHead>模型</TableHead></TableRow></TableHeader>
                <TableBody>
                  {checks.slice(0, 20).map((check) => (
                    <TableRow key={check.id}>
                      <TableCell className="whitespace-nowrap text-xs" title={dateTime(check.created_at)}>{relativeTime(check.created_at)}</TableCell>
                      <TableCell><Badge variant="outline">{check.level}</Badge></TableCell>
                      <TableCell><StatusBadge status={check.status} /></TableCell>
                      <TableCell className="tabular-nums">{formatLatency(check.latency_ms)}</TableCell>
                      <TableCell className="tabular-nums">{check.http_status || "-"}</TableCell>
                      <TableCell className="max-w-44 truncate text-xs" title={check.model || check.message}>{check.model || check.message || "-"}</TableCell>
                    </TableRow>
                  ))}
                  {checks.length === 0 ? <TableRow><TableCell colSpan={6} className="py-8 text-center text-sm text-muted-foreground">暂无探测记录</TableCell></TableRow> : null}
                </TableBody>
              </Table>
            </div>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}

function Stat({ label, value, tone }: { label: string; value: string; tone?: string }) {
  return <div className="rounded border bg-muted/10 p-3"><div className="text-xs text-muted-foreground">{label}</div><div className={cn("mt-2 text-2xl font-semibold tabular-nums", tone)}>{value}</div></div>
}

function StatusBadge({ status }: { status: string }) {
  if (status === "success") return <Badge variant="outline" className="border-emerald-300 text-emerald-700">正常</Badge>
  if (status === "skipped_budget") return <Badge variant="outline" className="border-amber-300 text-amber-700">跳过</Badge>
  return <Badge variant="destructive">{statusLabel(status)}</Badge>
}

function statusLabel(status: string) {
  if (status === "failure") return "失败"
  if (status === "config_error") return "配置错误"
  if (status === "skipped_budget") return "预算跳过"
  return status || "未知"
}

function formatPercent(value?: number | null) {
  return value == null || !Number.isFinite(value) ? "-" : `${value.toFixed(value >= 99.95 ? 2 : 1)}%`
}

function formatLatency(value?: number | null) {
  return value == null || !Number.isFinite(value) || value <= 0 ? "-" : `${Math.round(value)} ms`
}

function successTone(value?: number | null) {
  if (value == null) return ""
  if (value >= 99) return "text-emerald-600"
  if (value >= 95) return "text-amber-600"
  return "text-red-600"
}
