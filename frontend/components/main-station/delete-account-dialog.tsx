import { useEffect, useState } from "react"
import { Trash2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Label } from "@/components/ui/label"
import { Spinner } from "@/components/ui/spinner"
import type { MainStationAccount } from "@/lib/api-types"

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  account: MainStationAccount | null
  busy: boolean
  onConfirm: (deleteSourceAPIKey: boolean) => Promise<void>
}

export function DeleteAccountDialog({ open, onOpenChange, account, busy, onConfirm }: Props) {
  const [deleteSourceAPIKey, setDeleteSourceAPIKey] = useState(false)
  const sourceAPIKeyID = account?.member?.source_api_key_id
  const canDeleteSourceAPIKey = sourceAPIKeyID != null && sourceAPIKeyID > 0

  useEffect(() => {
    if (open) setDeleteSourceAPIKey(false)
  }, [account?.member?.id, open])

  return (
    <Dialog open={open} onOpenChange={(nextOpen) => { if (!busy) onOpenChange(nextOpen) }}>
      <DialogContent className="sm:max-w-md" showCloseButton={!busy}>
        <DialogHeader>
          <DialogTitle>{`删除账号“${account?.name ?? ""}”`}</DialogTitle>
          <DialogDescription>将删除主站账号和本地接管记录，此操作不可恢复。</DialogDescription>
        </DialogHeader>

        <div className="flex items-start gap-3 rounded-md border p-3">
          <Checkbox
            id="delete-source-api-key"
            className="mt-0.5"
            checked={deleteSourceAPIKey}
            disabled={!canDeleteSourceAPIKey || busy}
            onCheckedChange={(checked) => setDeleteSourceAPIKey(checked === true)}
          />
          <div className="min-w-0 space-y-1">
            <Label htmlFor="delete-source-api-key">同时删除上游 API Key</Label>
            <p className="text-xs leading-5 text-muted-foreground">
              {canDeleteSourceAPIKey
                ? `精确删除当前绑定的 Key #${sourceAPIKeyID}；未勾选时上游 Key 保持不变。`
                : "该账号没有可精确匹配的上游 Key，无法联动删除。"}
            </p>
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" disabled={busy} onClick={() => onOpenChange(false)}>取消</Button>
          <Button variant="destructive" disabled={busy || !account?.member} onClick={() => void onConfirm(deleteSourceAPIKey)}>
            {busy ? <Spinner /> : <Trash2 />}
            {deleteSourceAPIKey ? "全部删除" : "删除主站账号"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
