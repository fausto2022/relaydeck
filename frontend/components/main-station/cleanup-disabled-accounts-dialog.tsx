import { useEffect, useState } from "react"
import { AlertTriangle, Trash2 } from "lucide-react"
import { toast } from "sonner"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Spinner } from "@/components/ui/spinner"
import { apiFetch } from "@/lib/api"
import type {
  MainStationDisabledCleanupPreview,
  MainStationDisabledCleanupResult,
  MainStationGroupWorkspace,
} from "@/lib/api-types"
import { dateTime } from "@/lib/format"

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  workspace: MainStationGroupWorkspace | null
  onCleaned: () => void
}

export function CleanupDisabledAccountsDialog({ open, onOpenChange, workspace, onCleaned }: Props) {
  const [preview, setPreview] = useState<MainStationDisabledCleanupPreview | null>(null)
  const [loading, setLoading] = useState(false)
  const [cleaning, setCleaning] = useState(false)
  const [error, setError] = useState("")

  useEffect(() => {
    if (!open || !workspace) return
    let active = true
    setLoading(true)
    setPreview(null)
    setError("")
    apiFetch<MainStationDisabledCleanupPreview>(`/main-station/groups/${workspace.group.id}/accounts/disabled-cleanup`)
      .then((result) => { if (active) setPreview(result) })
      .catch((loadError) => { if (active) setError(loadError instanceof Error ? loadError.message : "加载清理预览失败") })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false }
  }, [open, workspace])

  async function handleCleanup() {
    if (!workspace || !preview?.eligible) return
    setCleaning(true)
    try {
      const result = await apiFetch<MainStationDisabledCleanupResult>(`/main-station/groups/${workspace.group.id}/accounts/disabled-cleanup`, {
        method: "POST",
        body: JSON.stringify({ confirm: true }),
      })
      onCleaned()
      if (result.errors.length > 0) {
        setError(result.errors.join("；"))
        toast.warning(`已清理 ${result.deleted} 个，${result.errors.length} 个失败`)
        return
      }
      toast.success(`已清理 ${result.deleted} 个停用账号及其上游 Key`)
      onOpenChange(false)
    } catch (cleanupError) {
      setError(cleanupError instanceof Error ? cleanupError.message : "清理停用账号失败")
    } finally {
      setCleaning(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(nextOpen) => { if (!cleaning) onOpenChange(nextOpen) }}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>清理停用账号</DialogTitle>
          <DialogDescription>{workspace?.group.name ?? "当前分组"}</DialogDescription>
        </DialogHeader>

        <Alert variant="destructive">
          <AlertTriangle className="size-4" />
          <AlertTitle>此操作不可恢复</AlertTitle>
          <AlertDescription>确认后会同时删除主站 Account、精确绑定的上游 API Key 和本地接管记录。没有完整 Account ID 或 Key ID 的账号不会处理。</AlertDescription>
        </Alert>

        {loading ? <div className="flex min-h-32 items-center justify-center"><Spinner /></div> : null}
        {error ? <p className="text-sm text-destructive">{error}</p> : null}
        {!loading && preview ? (
          <div className="space-y-3">
            <div className="flex items-center justify-between text-sm">
              <span>可清理 <strong>{preview.eligible}</strong> 个</span>
              <span className="text-muted-foreground">跳过 {preview.skipped} 个</span>
            </div>
            <p className="text-xs text-muted-foreground">跳过项包括未持续停用、缺少精确 ID，或上游 Key 仍被其他主站账号使用的记录。</p>
            {preview.candidates.length > 0 ? (
              <div className="max-h-64 divide-y overflow-y-auto border">
                {preview.candidates.map((candidate) => (
                  <div key={candidate.member_id} className="space-y-1 px-3 py-2.5">
                    <p className="truncate text-sm font-medium" title={candidate.account_name}>{candidate.account_name || `账号 #${candidate.remote_account_id}`}</p>
                    <p className="text-xs text-muted-foreground">主站 #{candidate.remote_account_id} · 上游 Key #{candidate.source_api_key_id}{candidate.source_api_key_name ? ` · ${candidate.source_api_key_name}` : ""}</p>
                    <p className="text-xs text-muted-foreground">停用于 {dateTime(candidate.disabled_since)}</p>
                  </div>
                ))}
              </div>
            ) : <p className="py-5 text-center text-sm text-muted-foreground">当前没有符合条件的停用账号</p>}
          </div>
        ) : null}

        <DialogFooter>
          <Button variant="outline" disabled={cleaning} onClick={() => onOpenChange(false)}>取消</Button>
          <Button variant="destructive" disabled={loading || cleaning || !preview?.eligible} onClick={() => void handleCleanup()}>
            {cleaning ? <Spinner /> : <Trash2 className="size-4" />}
            {cleaning ? "清理中" : `清理 ${preview?.eligible ?? 0} 个账号`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
