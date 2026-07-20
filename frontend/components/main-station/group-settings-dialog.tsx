import { useEffect, useState } from "react"
import { Save, Sparkles } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { apiFetch } from "@/lib/api"
import type { MainStationConfig, MainStationGroupWorkspace } from "@/lib/api-types"

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  workspace: MainStationGroupWorkspace | null
  config: MainStationConfig | null
  onSaved: (workspace: MainStationGroupWorkspace) => void
}

export function GroupSettingsDialog({ open, onOpenChange, workspace, config, onSaved }: Props) {
  const [enabled, setEnabled] = useState(true)
  const [minimumHealthy, setMinimumHealthy] = useState(1)
  const [minimumConcurrency, setMinimumConcurrency] = useState(1)
  const [rateSortDirection, setRateSortDirection] = useState<"asc" | "desc" | "stability">("asc")
  const [healthPolicy, setHealthPolicy] = useState("")
  const [marginPolicy, setMarginPolicy] = useState("")
  const [rankingIntervalSeconds, setRankingIntervalSeconds] = useState(0)
  const [minimumMarginPercent, setMinimumMarginPercent] = useState("")
  const [autoExpandEnabled, setAutoExpandEnabled] = useState(false)
  const [autoExpandMinMarginPercent, setAutoExpandMinMarginPercent] = useState(0)
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!open || !workspace) return
    setEnabled(workspace.enabled)
    setMinimumHealthy(workspace.minimum_healthy_accounts)
    setMinimumConcurrency(workspace.minimum_effective_concurrency)
    setRateSortDirection(workspace.rate_sort_direction)
    setHealthPolicy(workspace.health_policy)
    setMarginPolicy(workspace.margin_policy)
    setRankingIntervalSeconds(workspace.ranking_interval_seconds ?? 0)
    setMinimumMarginPercent(workspace.minimum_margin_basis_points == null ? "" : String(workspace.minimum_margin_basis_points / 100))
    setAutoExpandEnabled(workspace.auto_expand_enabled ?? false)
    setAutoExpandMinMarginPercent((workspace.auto_expand_min_margin_basis_points ?? 0) / 100)
    setAdvancedOpen(false)
  }, [open, workspace])

  async function handleSave() {
    if (!workspace) return
    if (rankingIntervalSeconds !== 0 && (rankingIntervalSeconds < 5 || rankingIntervalSeconds > 86400)) {
      toast.error("分组重排间隔必须为 0，或在 5 到 86400 秒之间")
      return
    }
    if (!Number.isFinite(autoExpandMinMarginPercent) || autoExpandMinMarginPercent < 0 || autoExpandMinMarginPercent > 99) {
      toast.error("自动扩池最低利润率必须在 0% 到 99% 之间")
      return
    }
    const minimumMarginValue = minimumMarginPercent.trim() === "" ? null : Number(minimumMarginPercent)
    if (minimumMarginValue != null && (!Number.isFinite(minimumMarginValue) || minimumMarginValue < 0 || minimumMarginValue > 99)) {
      toast.error("本分组最低利润率必须在 0% 到 99% 之间")
      return
    }
    setBusy(true)
    try {
      const saved = await apiFetch<MainStationGroupWorkspace>(`/main-station/groups/${workspace.group.id}/settings`, {
        method: "PUT",
        body: JSON.stringify({
          enabled,
          minimum_healthy_accounts: minimumHealthy,
          minimum_effective_concurrency: minimumConcurrency,
          rate_sort_direction: rateSortDirection,
          health_policy: healthPolicy,
          margin_policy: marginPolicy,
          ranking_interval_seconds: rankingIntervalSeconds,
          minimum_margin_basis_points: minimumMarginValue == null ? null : Math.round(minimumMarginValue * 100),
          auto_expand_enabled: autoExpandEnabled,
          auto_expand_min_margin_basis_points: Math.round(autoExpandMinMarginPercent * 100),
        }),
      })
      onSaved(saved)
      onOpenChange(false)
      toast.success("分组设置已保存")
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "保存分组设置失败")
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>分组设置</DialogTitle>
          <DialogDescription>{workspace?.group.name}</DialogDescription>
        </DialogHeader>

        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="minimum-healthy">最少可用账号</Label>
            <Input id="minimum-healthy" type="number" min={0} value={minimumHealthy} onChange={(event) => setMinimumHealthy(Number(event.target.value))} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="minimum-concurrency">最少有效并发</Label>
            <Input id="minimum-concurrency" type="number" min={0} value={minimumConcurrency} onChange={(event) => setMinimumConcurrency(Number(event.target.value))} />
          </div>
          <div className="space-y-2 sm:col-span-2">
            <Label>调度排序</Label>
            <Select value={rateSortDirection} onValueChange={(value) => setRateSortDirection(value as "asc" | "desc" | "stability")}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="asc">低成本优先</SelectItem>
                <SelectItem value="desc">高成本优先</SelectItem>
                <SelectItem value="stability">稳定性优先</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="flex items-center justify-between border-t pt-4 sm:col-span-2">
            <Label htmlFor="group-enabled">启用分组调度</Label>
            <Switch id="group-enabled" checked={enabled} onCheckedChange={setEnabled} />
          </div>
          <div className="space-y-2 sm:col-span-2">
            <Label htmlFor="group-ranking-interval">本分组重排间隔（秒）</Label>
            <Input
              id="group-ranking-interval"
              type="number"
              min={0}
              max={86400}
              value={rankingIntervalSeconds}
              onChange={(event) => setRankingIntervalSeconds(Number(event.target.value))}
            />
            <p className="text-xs text-muted-foreground">填 0 继承主站全局设置；最小自定义间隔为 5 秒。</p>
          </div>
          <div className="space-y-2 border-t pt-4 sm:col-span-2">
            <Label htmlFor="group-minimum-margin">本分组最低利润率（%）</Label>
            <Input
              id="group-minimum-margin"
              type="number"
              min={0}
              max={99}
              step={0.1}
              value={minimumMarginPercent}
              placeholder={`继承全局（${((config?.minimum_margin_basis_points ?? 0) / 100).toFixed(1)}%）`}
              onChange={(event) => setMinimumMarginPercent(event.target.value)}
            />
            <p className="text-xs text-muted-foreground">留空继承全局；实际利润率等于最低要求时仍算正常，低于要求才判定利润不足。</p>
          </div>
          <div className="space-y-4 border-t pt-4 sm:col-span-2">
            <div className="flex items-center justify-between gap-4">
              <div className="min-w-0">
                <Label htmlFor="group-auto-expand" className="flex items-center gap-2">
                  <Sparkles className="size-4" />自动扩池
                </Label>
                <p className="mt-1 text-xs text-muted-foreground">倍率同步后自动筛选同类型上游分组。</p>
              </div>
              <Switch id="group-auto-expand" checked={autoExpandEnabled} onCheckedChange={setAutoExpandEnabled} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="group-auto-expand-margin">自动扩池最低利润率（%）</Label>
              <Input
                id="group-auto-expand-margin"
                type="number"
                min={0}
                max={99}
                step={0.1}
                value={autoExpandMinMarginPercent}
                disabled={!autoExpandEnabled}
                onChange={(event) => setAutoExpandMinMarginPercent(Number(event.target.value))}
              />
              <p className="text-xs text-muted-foreground">利润率必须严格高于该值；每轮最多测试 3 个候选，连续 3 次通过后最多新增 1 个账号。</p>
            </div>
            {workspace?.last_auto_expand_at || workspace?.last_auto_expand_error ? (
              <div className="text-xs text-muted-foreground">
                {workspace.last_auto_expand_at ? `最近执行：${new Date(workspace.last_auto_expand_at).toLocaleString("zh-CN")}` : "尚未执行"}
                {workspace.last_auto_expand_error ? <p className="mt-1 text-destructive">最近错误：{workspace.last_auto_expand_error}</p> : null}
              </div>
            ) : null}
          </div>
        </div>

        <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
          <CollapsibleTrigger asChild>
            <Button type="button" variant="ghost" className="w-full justify-start px-0">高级保护策略</Button>
          </CollapsibleTrigger>
          <CollapsibleContent className="space-y-4 pt-2">
            <div className="space-y-2">
              <Label htmlFor="health-policy">健康策略 JSON</Label>
              <Textarea id="health-policy" value={healthPolicy} onChange={(event) => setHealthPolicy(event.target.value)} rows={5} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="margin-policy">利润策略 JSON</Label>
              <Textarea id="margin-policy" value={marginPolicy} onChange={(event) => setMarginPolicy(event.target.value)} rows={4} />
            </div>
          </CollapsibleContent>
        </Collapsible>

        <DialogFooter>
          <Button onClick={handleSave} disabled={busy || !workspace}>
            <Save className="size-4" />{busy ? "保存中" : "保存"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
