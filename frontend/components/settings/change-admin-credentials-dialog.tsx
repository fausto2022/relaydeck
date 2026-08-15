import { useEffect, useState, type FormEvent } from "react"
import { Eye, EyeOff } from "lucide-react"
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
import { Button } from "@/components/ui/button"
import { apiFetch } from "@/lib/api"

interface ChangeAdminCredentialsDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentUsername: string
  onSuccess: () => void
}

interface PasswordFieldProps {
  id: string
  label: string
  value: string
  autoComplete: string
  disabled: boolean
  onChange: (value: string) => void
}

export function ChangeAdminCredentialsDialog({
  open,
  onOpenChange,
  currentUsername,
  onSuccess,
}: ChangeAdminCredentialsDialogProps) {
  const [currentPassword, setCurrentPassword] = useState("")
  const [username, setUsername] = useState(currentUsername)
  const [newPassword, setNewPassword] = useState("")
  const [confirmPassword, setConfirmPassword] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!open) return
    setCurrentPassword("")
    setUsername(currentUsername)
    setNewPassword("")
    setConfirmPassword("")
    setError(null)
  }, [open, currentUsername])

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const nextUsername = username.trim()
    if (!nextUsername) {
      setError("管理员账号不能为空")
      return
    }
    if (newPassword.length < 8) {
      setError("新密码至少需要 8 位")
      return
    }
    if (newPassword !== confirmPassword) {
      setError("两次输入的新密码不一致")
      return
    }

    setSubmitting(true)
    setError(null)
    try {
      await apiFetch<{ username: string; message: string }>("/auth/change-credentials", {
        method: "POST",
        skipAuthErrorHandler: true,
        body: JSON.stringify({
          current_password: currentPassword,
          username: nextUsername,
          new_password: newPassword,
        }),
      })
      onOpenChange(false)
      onSuccess()
    } catch (err) {
      setError(err instanceof Error ? err.message : "修改失败")
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>修改管理员账号密码</DialogTitle>
          <DialogDescription>
            修改成功后当前登录会失效，需要使用新账号密码重新登录。
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="admin-credentials-username">管理员账号</Label>
            <Input
              id="admin-credentials-username"
              value={username}
              autoComplete="username"
              onChange={(event) => setUsername(event.target.value)}
              disabled={submitting}
              required
            />
          </div>

          <PasswordField
            id="admin-credentials-current-password"
            label="当前密码"
            value={currentPassword}
            autoComplete="current-password"
            disabled={submitting}
            onChange={setCurrentPassword}
          />
          <PasswordField
            id="admin-credentials-new-password"
            label="新密码"
            value={newPassword}
            autoComplete="new-password"
            disabled={submitting}
            onChange={setNewPassword}
          />
          <PasswordField
            id="admin-credentials-confirm-password"
            label="确认新密码"
            value={confirmPassword}
            autoComplete="new-password"
            disabled={submitting}
            onChange={setConfirmPassword}
          />
          <p className="text-xs leading-5 text-muted-foreground">新密码至少 8 位，建议使用大小写字母、数字和符号组合。</p>

          {error ? (
            <p className="text-sm text-destructive" role="alert" aria-live="polite">
              {error}
            </p>
          ) : null}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={submitting}>
              取消
            </Button>
            <Button type="submit" disabled={submitting || !currentPassword || !username.trim() || !newPassword || !confirmPassword}>
              {submitting ? "保存中…" : "保存并重新登录"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function PasswordField({ id, label, value, autoComplete, disabled, onChange }: PasswordFieldProps) {
  const [visible, setVisible] = useState(false)

  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      <div className="relative">
        <Input
          id={id}
          type={visible ? "text" : "password"}
          value={value}
          autoComplete={autoComplete}
          onChange={(event) => onChange(event.target.value)}
          className="pr-11"
          disabled={disabled}
          required
        />
        <button
          type="button"
          className="absolute right-1 top-1/2 flex size-10 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          onClick={() => setVisible((current) => !current)}
          aria-label={visible ? `隐藏${label}` : `显示${label}`}
          disabled={disabled}
        >
          {visible ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
        </button>
      </div>
    </div>
  )
}
