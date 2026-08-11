"use client"

import { useCallback, useEffect, useMemo, useState } from "react"
import {
  AlertTriangle,
  CheckCircle2,
  KeyRound,
  Link2,
  Loader2,
  Plus,
  TestTubeDiagonal,
  Unlink,
  XCircle,
} from "lucide-react"
import { toast } from "sonner"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { useConfirm } from "@/components/ui/confirm-dialog"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { apiFetch, type ApiError } from "@/lib/api"
import { channelTypeLabel, formatRatio } from "@/lib/format"
import {
  mainStationHealthAPIMode,
  mainStationPlatformsMatch,
  normalizeMainStationPlatform,
} from "@/lib/main-station-platform"
import { isImageQuickTestModel } from "@/lib/rate-ranking"
import { cn } from "@/lib/utils"
import type {
  Channel,
  MainStationConfig,
  MainStationGroupWorkspace,
  MainStationHealthModelCatalog,
  MainStationMember,
  MainStationRateUsage,
  MainStationRateUsageAccount,
  RateQuickTestResult,
  RateSnapshot,
} from "@/lib/api-types"

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  channel: Channel | null
  rate: RateSnapshot | null
  onChanged: () => void
}

export function ChannelGroupUsageDialog({ open, onOpenChange, channel, rate, onChanged }: Props) {
  const { confirm, dialog: confirmDialog } = useConfirm()
  const [usage, setUsage] = useState<MainStationRateUsage | null>(null)
  const [config, setConfig] = useState<MainStationConfig | null>(null)
  const [catalogs, setCatalogs] = useState<MainStationHealthModelCatalog[]>([])
  const [workspaces, setWorkspaces] = useState<MainStationGroupWorkspace[]>([])
  const [targetGroupID, setTargetGroupID] = useState("")
  const [model, setModel] = useState("")
  const [loading, setLoading] = useState(false)
  const [action, setAction] = useState<"test-only" | "test-add" | "direct" | null>(null)
  const [error, setError] = useState("")
  const [testResult, setTestResult] = useState<RateQuickTestResult | null>(null)

  const platform = normalizeMainStationPlatform(rate?.platform || rate?.ranking_provider)
  const connectedGroupIDs = useMemo(
    () => new Set((usage?.groups ?? []).filter((group) => group.connected).map((group) => group.group_id)),
    [usage],
  )
  const availableWorkspaces = useMemo(() => workspaces
    .filter((workspace) => workspace.enabled
      && !workspace.group.missing
      && workspace.group.status.toLowerCase() === "active"
      && !connectedGroupIDs.has(workspace.group.id)
      && mainStationPlatformsMatch(workspace.group.platform, platform))
    .sort((left, right) => left.group.sort - right.group.sort || left.group.id - right.group.id),
  [connectedGroupIDs, platform, workspaces])
  const modelOptions = useMemo(() => {
    const configured = config?.health_models?.[platform] ?? ""
    const discovered = catalogs.find((item) => normalizeMainStationPlatform(item.platform) === platform)?.models ?? []
    return Array.from(new Set([configured, ...discovered].map((item) => item.trim()).filter(Boolean)))
  }, [catalogs, config, platform])

  const loadUsage = useCallback(async () => {
    if (!channel || !rate) return
    const result = await apiFetch<MainStationRateUsage>(
      `/channels/${channel.id}/rates/${rate.id}/main-station-usage`,
    )
    setUsage(result)
  }, [channel, rate])

  useEffect(() => {
    if (!open || !channel || !rate) return
    let cancelled = false
    setLoading(true)
    setError("")
    setTestResult(null)
    setUsage(null)
    setConfig(null)
    setCatalogs([])
    setWorkspaces([])
    setTargetGroupID("")
    setModel("")
    Promise.allSettled([
      apiFetch<MainStationRateUsage>(`/channels/${channel.id}/rates/${rate.id}/main-station-usage`),
      apiFetch<MainStationConfig>("/main-station"),
      apiFetch<MainStationHealthModelCatalog[]>("/main-station/health-models"),
      apiFetch<{ items: MainStationGroupWorkspace[] }>("/main-station/groups"),
    ]).then(([usageResult, configResult, catalogsResult, workspacesResult]) => {
      if (cancelled) return
      if (usageResult.status === "fulfilled") setUsage(usageResult.value)
      if (configResult.status === "fulfilled") setConfig(configResult.value)
      if (catalogsResult.status === "fulfilled") setCatalogs(catalogsResult.value)
      if (workspacesResult.status === "fulfilled") setWorkspaces(workspacesResult.value.items)
      const messages = [usageResult, configResult, catalogsResult, workspacesResult]
        .filter((result) => result.status === "rejected")
        .map((result) => result.status === "rejected" && result.reason instanceof Error
          ? result.reason.message
          : "读取主站信息失败")
      setError(messages.join("；"))
    }).finally(() => {
      if (!cancelled) setLoading(false)
    })
    return () => { cancelled = true }
  }, [channel, open, rate])

  useEffect(() => {
    if (targetGroupID && availableWorkspaces.some((item) => String(item.group.id) === targetGroupID)) return
    setTargetGroupID(availableWorkspaces[0] ? String(availableWorkspaces[0].group.id) : "")
  }, [availableWorkspaces, targetGroupID])

  useEffect(() => {
    if (model || !platform) return
    const configured = config?.health_models?.[platform]?.trim()
    const discovered = catalogs.find((item) => normalizeMainStationPlatform(item.platform) === platform)?.models[0]?.trim()
    setModel(configured || discovered || "")
  }, [catalogs, config, model, platform])

  async function createManagedAccount(allowNameConflict = false) {
    if (!channel || !rate || !targetGroupID) return false
    const workspace = availableWorkspaces.find((item) => String(item.group.id) === targetGroupID)
    if (!workspace) {
      toast.error("请选择可用的主站分组")
      return false
    }
    const imageMode = isImageQuickTestModel(model) || platform === "image"
    try {
      await apiFetch<MainStationMember>(`/main-station/groups/${workspace.group.id}/accounts`, {
        method: "POST",
        body: JSON.stringify({
          account_name: rate.model_name,
          ownership_mode: "managed",
          source_channel_id: channel.id,
          source_group_id: rate.remote_group_id ?? undefined,
          source_group_name: rate.model_name,
          allow_name_conflict: allowNameConflict,
          enabled: true,
          preferred: false,
          priority: 1,
          concurrency: 0,
          rate_convert_mode: "raw",
          rate_convert_value: 1,
          cost_adjustment: 1,
          health_enabled: true,
          health_model: model.trim(),
          health_api_mode: mainStationHealthAPIMode(workspace.group.platform, imageMode),
          initialize_async: true,
        }),
      })
      toast.success(`已加入主站分组「${workspace.group.name}」，账号正在初始化`)
      await loadUsage()
      onChanged()
      return true
    } catch (caught) {
      if (!allowNameConflict && isManagedAccountNameConflict(caught)) {
        const approved = await confirm({
          title: "主站已存在同名账号",
          description: "继续后会创建独立 Key 和另一条同名账号，不会覆盖现有账号。",
          confirmLabel: "继续添加",
          cancelLabel: "取消",
        })
        if (approved) return createManagedAccount(true)
        return false
      }
      throw caught
    }
  }

  async function runQuickTest() {
    if (!channel || !rate || !model.trim()) return null
    const imageMode = isImageQuickTestModel(model) || platform === "image"
    const result = await apiFetch<RateQuickTestResult>(`/channels/${channel.id}/rates/${rate.id}/test`, {
      method: "POST",
      body: JSON.stringify({ platform, model: model.trim(), mode: imageMode ? "image" : "chat" }),
    })
    setTestResult(result)
    return result
  }

  async function testOnly() {
    if (!channel || !rate || !model.trim()) return
    setAction("test-only")
    setTestResult(null)
    try {
      const result = await runQuickTest()
      if (result?.usable) toast.success("快速测试通过，未加入主站")
      else if (result) toast.warning(result.message)
    } catch (caught) {
      toast.error(caught instanceof Error ? caught.message : "快速测试失败")
    } finally {
      setAction(null)
    }
  }

  async function testAndAdd() {
    if (!channel || !rate || !targetGroupID || !model.trim()) return
    setAction("test-add")
    setTestResult(null)
    try {
      const result = await runQuickTest()
      if (!result?.usable) {
        if (result) toast.warning(result.message)
        return
      }
      await createManagedAccount()
    } catch (caught) {
      toast.error(caught instanceof Error ? caught.message : "测试并加入失败")
    } finally {
      setAction(null)
    }
  }

  async function directAdd() {
    if (!targetGroupID) return
    const approved = await confirm({
      title: "不测试直接加入？",
      description: "将立即创建专用 Key 和主站账号，后续由账号探活验证可用性。",
      confirmLabel: "直接加入",
      cancelLabel: "取消",
    })
    if (!approved) return
    setAction("direct")
    setTestResult(null)
    try {
      await createManagedAccount()
    } catch (caught) {
      toast.error(caught instanceof Error ? caught.message : "加入主站失败")
    } finally {
      setAction(null)
    }
  }

  const testDisabled = loading || action !== null || !model.trim() || !platform
  const addDisabled = loading || action !== null || !targetGroupID

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="max-h-[calc(100dvh-1rem)] overflow-y-auto sm:max-w-4xl">
          <DialogHeader>
            <DialogTitle className="flex flex-wrap items-center gap-2 text-base">
              <span>{rate?.model_name || "上游分组"}</span>
              {rate ? <Badge variant="outline" className="tabular-nums">倍率 {formatRatio(rate.ratio)}</Badge> : null}
            </DialogTitle>
            <DialogDescription>
              {channel ? `${channel.name} · ${channelTypeLabel(channel.type)}` : "渠道分组主站使用情况"}
            </DialogDescription>
          </DialogHeader>

          {loading ? (
            <div className="flex h-48 items-center justify-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="size-4 animate-spin" />加载使用情况
            </div>
          ) : error && !usage ? (
            <div className="flex h-48 items-center justify-center text-sm text-destructive">{error}</div>
          ) : (
            <div className="space-y-5">
              <UsageSummary usage={usage} />

              {error ? <p className="text-sm text-destructive">{error}</p> : null}

              <section>
                <div className="mb-2 flex items-center justify-between gap-3">
                  <h3 className="text-sm font-semibold">主站账号明细</h3>
                  <span className="text-xs text-muted-foreground">{usage?.account_count ?? 0} 个账号</span>
                </div>
                <UsageTable usage={usage} />
              </section>

              <section className="border-t border-border pt-4">
                <div className="mb-3 flex items-center justify-between gap-3">
                  <h3 className="text-sm font-semibold">加入主站分组</h3>
                  {availableWorkspaces.length === 0 ? (
                    <span className="text-xs text-muted-foreground">没有未接入的同类型分组</span>
                  ) : null}
                </div>
                <div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] lg:items-end">
                  <div className="space-y-1.5">
                    <Label htmlFor="channel-rate-target-group">目标主站分组</Label>
                    <Select value={targetGroupID} onValueChange={setTargetGroupID} disabled={action !== null || availableWorkspaces.length === 0}>
                      <SelectTrigger id="channel-rate-target-group" className="w-full"><SelectValue placeholder="选择主站分组" /></SelectTrigger>
                      <SelectContent>
                        {availableWorkspaces.map((workspace) => (
                          <SelectItem key={workspace.group.id} value={String(workspace.group.id)}>
                            {workspace.group.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor="channel-rate-health-model">测试及探活模型</Label>
                    <Select value={model} onValueChange={(value) => {
                      setModel(value)
                      setTestResult(null)
                    }} disabled={action !== null || modelOptions.length === 0}>
                      <SelectTrigger id="channel-rate-health-model" className="w-full">
                        <SelectValue placeholder={loading ? "加载模型中" : "选择测试模型"} />
                      </SelectTrigger>
                      <SelectContent>
                        {modelOptions.map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="grid gap-2 sm:grid-cols-3 lg:flex lg:justify-end">
                    <Button variant="outline" className="w-full lg:w-auto" onClick={() => void testOnly()} disabled={testDisabled}>
                      {action === "test-only" ? <Loader2 className="animate-spin" /> : <TestTubeDiagonal />}
                      仅测试
                    </Button>
                    <Button variant="outline" className="w-full lg:w-auto" onClick={() => void directAdd()} disabled={addDisabled}>
                      {action === "direct" ? <Loader2 className="animate-spin" /> : <Plus />}
                      直接加入
                    </Button>
                    <Button className="w-full lg:w-auto" onClick={() => void testAndAdd()} disabled={testDisabled || !targetGroupID}>
                      {action === "test-add" ? <Loader2 className="animate-spin" /> : <TestTubeDiagonal />}
                      测试并加入
                    </Button>
                  </div>
                </div>
                {testResult ? <QuickTestResult result={testResult} /> : null}
              </section>
            </div>
          )}
        </DialogContent>
      </Dialog>
      {confirmDialog}
    </>
  )
}

function QuickTestResult({ result }: { result: RateQuickTestResult }) {
  const status = result.usable
    ? { label: "测试通过", icon: CheckCircle2, className: "text-emerald-700 dark:text-emerald-300" }
    : result.reachable
      ? { label: "可连接但不可用", icon: AlertTriangle, className: "text-amber-700 dark:text-amber-300" }
      : { label: "测试失败", icon: XCircle, className: "text-destructive" }
  const StatusIcon = status.icon
  const tokenCount = result.total_tokens ?? ((result.input_tokens ?? 0) + (result.output_tokens ?? 0) || undefined)

  return (
    <div className="mt-4 space-y-3 border-y border-border py-4">
      <div className={cn("flex items-start gap-2.5", status.className)}>
        <StatusIcon className="mt-0.5 size-4 shrink-0" />
        <div className="min-w-0">
          <p className="text-sm font-semibold">{status.label}</p>
          <p className="mt-0.5 break-words text-sm text-foreground">{result.message}</p>
        </div>
      </div>
      <div className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm sm:grid-cols-4">
        <ResultMetric label="实际模型" value={result.model || "-"} />
        <ResultMetric label="平均延迟" value={`${result.latency_ms} ms`} />
        <ResultMetric label="HTTP 状态" value={result.http_status ? String(result.http_status) : "-"} />
        <ResultMetric label="Token" value={tokenCount == null ? "-" : String(tokenCount)} />
      </div>
      {result.image_url ? (
        <div className="overflow-hidden rounded-md border border-border bg-muted/30">
          <img src={result.image_url} alt="快速测试生成结果" className="mx-auto max-h-80 w-auto max-w-full object-contain" />
        </div>
      ) : null}
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <span>临时 Key：{result.temporary_key_name}</span>
        <Badge variant="outline" className={result.temporary_key_status === "deleted" ? "text-emerald-700 dark:text-emerald-300" : "text-amber-700 dark:text-amber-300"}>
          {result.temporary_key_status === "deleted" ? "已删除" : "等待后台清理"}
        </Badge>
        {result.cleanup_error ? <span className="text-amber-700 dark:text-amber-300">{result.cleanup_error}</span> : null}
      </div>
    </div>
  )
}

function ResultMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-0.5 truncate font-medium tabular-nums" title={value}>{value}</p>
    </div>
  )
}

function UsageSummary({ usage }: { usage: MainStationRateUsage | null }) {
  const state = usageState(usage)
  const Icon = state.icon
  const names = (usage?.groups ?? []).filter((group) => group.connected).map((group) => group.group_name)
  return (
    <div className={cn("flex items-start justify-between gap-4 border-y border-border py-3", state.className)}>
      <div className="flex min-w-0 items-start gap-2.5">
        <Icon className="mt-0.5 size-4 shrink-0" />
        <div className="min-w-0">
          <p className="text-sm font-medium">{state.label}</p>
          <p className="mt-0.5 truncate text-xs text-muted-foreground" title={names.join("、")}>{names.length ? names.join("、") : "尚未绑定到主站分组"}</p>
        </div>
      </div>
      <Badge variant="outline" className="tabular-nums">{usage?.account_count ?? 0} 个账号</Badge>
    </div>
  )
}

function UsageTable({ usage }: { usage: MainStationRateUsage | null }) {
  if (!usage || usage.groups.length === 0) {
    return <div className="rounded-md border border-dashed border-border px-4 py-8 text-center text-sm text-muted-foreground">未被 RelayDeck 主站绑定</div>
  }
  return (
    <div className="overflow-x-auto rounded-md border border-border">
      <table className="w-full min-w-[700px] text-sm">
        <thead className="bg-muted/40 text-xs text-muted-foreground">
          <tr>
            <th className="px-3 py-2 text-left font-medium">主站分组</th>
            <th className="px-3 py-2 text-left font-medium">主站账号</th>
            <th className="px-3 py-2 text-left font-medium">上游 Key</th>
            <th className="px-3 py-2 text-left font-medium">账号状态</th>
            <th className="px-3 py-2 text-left font-medium">探活</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {usage.groups.flatMap((group) => group.accounts.map((account) => (
            <tr key={`${group.group_id}-${account.member_id}`}>
              <td className="px-3 py-2.5">
                <div className="flex items-center gap-1.5">
                  <span>{group.group_name}</span>
                  {group.missing ? <Badge variant="destructive">已失效</Badge> : null}
                </div>
              </td>
              <td className="px-3 py-2.5">{account.main_account_name || `成员 #${account.member_id}`}</td>
              <td className="px-3 py-2.5">
                <div className="flex items-center gap-1.5">
                  <KeyRound className="size-3.5 text-muted-foreground" />
                  <span>{account.source_api_key_name || (account.source_api_key_id ? `#${account.source_api_key_id}` : "待创建")}</span>
                  {account.source_api_key_managed ? <Badge variant="outline">托管</Badge> : null}
                </div>
              </td>
              <td className="px-3 py-2.5"><AccountStatus account={account} /></td>
              <td className="px-3 py-2.5 text-muted-foreground">{healthLabel(account.last_health_status)}</td>
            </tr>
          ))) }
        </tbody>
      </table>
    </div>
  )
}

function AccountStatus({ account }: { account: MainStationRateUsageAccount }) {
  if (account.binding_status === "invalid" || account.binding_status === "orphaned" || account.status === "orphaned") {
    return <Badge variant="destructive">绑定异常</Badge>
  }
  if (account.status === "pending" || account.main_account_id == null) {
    return <Badge variant="outline" className="text-amber-700 dark:text-amber-300">初始化中</Badge>
  }
  if (!account.enabled || account.status === "disabled") {
    return <Badge variant="outline" className="text-muted-foreground">已停用</Badge>
  }
  if (account.status === "active") {
    return <Badge variant="outline" className="border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300">使用中</Badge>
  }
  return <Badge variant="outline" className="text-amber-700 dark:text-amber-300">{account.status || "异常"}</Badge>
}

function usageState(usage: MainStationRateUsage | null) {
  switch (usage?.status) {
    case "connected": return { label: "已接入主站", icon: Link2, className: "text-emerald-700 dark:text-emerald-300" }
    case "initializing": return { label: "主站账号初始化中", icon: Loader2, className: "text-amber-700 dark:text-amber-300" }
    case "abnormal": return { label: "存在失效绑定", icon: AlertTriangle, className: "text-destructive" }
    default: return { label: "未接入主站", icon: Unlink, className: "text-muted-foreground" }
  }
}

function healthLabel(status: string) {
  switch (status?.toLowerCase()) {
    case "success": return "正常"
    case "unhealthy": return "异常"
    case "config_error": return "配置错误"
    case "pending": return "等待检测"
    default: return status || "未检测"
  }
}

function isManagedAccountNameConflict(error: unknown) {
  const body = (error as ApiError | undefined)?.body as { code?: string } | undefined
  return body?.code === "managed_account_name_conflict"
}
