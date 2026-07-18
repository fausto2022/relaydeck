import { useEffect, useState } from "react"
import { Save } from "lucide-react"
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
import type { MainStationGroupWorkspace } from "@/lib/api-types"

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  workspace: MainStationGroupWorkspace | null
  onSaved: (workspace: MainStationGroupWorkspace) => void
}

export function GroupSettingsDialog({ open, onOpenChange, workspace, onSaved }: Props) {
  const [enabled, setEnabled] = useState(true)
  const [minimumHealthy, setMinimumHealthy] = useState(1)
  const [minimumConcurrency, setMinimumConcurrency] = useState(1)
  const [rateSortDirection, setRateSortDirection] = useState<"asc" | "desc" | "stability">("asc")
  const [healthPolicy, setHealthPolicy] = useState("")
  const [marginPolicy, setMarginPolicy] = useState("")
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
    setAdvancedOpen(false)
  }, [open, workspace])

  async function handleSave() {
    if (!workspace) return
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
