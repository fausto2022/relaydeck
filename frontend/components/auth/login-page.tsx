"use client"

import { useEffect, useRef, useState, type FormEvent } from "react"
import { Activity, AlertCircle, Eye, EyeOff, KeyRound, Loader2, LockKeyhole, UserRound } from "lucide-react"
import { Card, CardContent, CardHeader } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Button } from "@/components/ui/button"
import { useAuth } from "@/lib/auth-context"
import type { ApiError } from "@/lib/api"

export function LoginPage() {
  const { login } = useAuth()
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showPassword, setShowPassword] = useState(false)
  const passwordRef = useRef<HTMLInputElement>(null)
  useEffect(() => {
    document.title = "登录"
  }, [])

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setError(null)
    setSubmitting(true)
    try {
      await login(username.trim(), password)
    } catch (err) {
      const e = err as ApiError
      if (e.status === 401) {
        setError("账号或密码错误")
      } else {
        setError(e.message || "登录失败")
      }
      passwordRef.current?.focus()
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="flex min-h-dvh items-stretch bg-background">
      <div className="mx-auto grid w-full max-w-6xl lg:grid-cols-[minmax(0,1fr)_28rem]">
        <div className="hidden min-w-0 flex-col justify-between bg-sidebar px-12 py-12 text-sidebar-foreground lg:flex">
          <div className="flex items-center gap-3">
            <span className="flex size-10 items-center justify-center rounded-md border border-white/10 bg-sidebar-primary/15 text-sidebar-primary">
              <Activity className="size-5" strokeWidth={2.5} />
            </span>
            <div>
              <p className="text-base font-semibold">安全入口</p>
              <p className="text-[11px] text-sidebar-foreground/45">IDENTITY GATEWAY</p>
            </div>
          </div>
          <div className="max-w-md">
            <p className="text-[11px] font-semibold text-sidebar-primary">受保护的工作区</p>
            <p className="mt-3 text-3xl font-semibold leading-tight">登录以继续</p>
            <p className="mt-4 text-sm leading-6 text-sidebar-foreground/55">使用管理员凭据完成身份验证。</p>
          </div>
          <p className="text-[11px] text-sidebar-foreground/35">SESSION PROTECTED · ACCESS CONTROLLED</p>
        </div>

        <div className="flex items-center px-4 py-8 sm:px-10 lg:px-12">
        <Card className="dashboard-panel mx-auto w-full max-w-md gap-0 border-border/80 py-0">
          <CardHeader className="border-b border-border/60 px-5 py-5 sm:px-6">
            <div className="flex items-center gap-2.5">
              <span className="flex size-9 items-center justify-center rounded-md border border-primary/15 bg-primary/10 text-primary">
                <KeyRound className="size-4" />
              </span>
              <div>
                <h1 className="text-lg font-semibold leading-none">欢迎回来</h1>
                <p className="mt-1 text-xs text-muted-foreground">请输入登录信息</p>
              </div>
            </div>
          </CardHeader>
          <CardContent className="px-5 py-5 sm:px-6">
            <form onSubmit={handleSubmit} className="space-y-5">
              <div className="space-y-2">
                <Label htmlFor="username">账号</Label>
                <div className="relative">
                  <UserRound className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
                  <Input
                    id="username"
                    name="username"
                    autoComplete="username"
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    className="h-11 pl-10"
                    required
                    disabled={submitting}
                  />
                </div>
              </div>
              <div className="space-y-2">
                <Label htmlFor="password">密码</Label>
                <div className="relative">
                  <LockKeyhole className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
                  <Input
                    ref={passwordRef}
                    id="password"
                    name="password"
                    type={showPassword ? "text" : "password"}
                    autoComplete="current-password"
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    className="h-11 pl-10 pr-11"
                    required
                    disabled={submitting}
                    aria-invalid={error ? true : undefined}
                    aria-describedby={error ? "login-error" : undefined}
                  />
                  <button
                    type="button"
                    className="absolute right-1 top-1/2 flex size-10 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    onClick={() => setShowPassword((visible) => !visible)}
                    aria-label={showPassword ? "隐藏密码" : "显示密码"}
                    disabled={submitting}
                  >
                    {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
                  </button>
                </div>
                {error ? (
                  <p id="login-error" className="flex items-start gap-2 rounded-md border border-danger/25 bg-danger/5 px-3 py-2.5 text-xs leading-5 text-danger" role="alert" aria-live="polite">
                    <AlertCircle className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
                    <span>{error}，请检查后重试。</span>
                  </p>
                ) : null}
              </div>
              <Button type="submit" className="h-11 w-full" disabled={submitting}>
                {submitting ? <Loader2 className="size-4 animate-spin" /> : null}
                {submitting ? "登录中…" : "登录控制台"}
              </Button>
            </form>
          </CardContent>
        </Card>
        </div>
      </div>
    </div>
  )
}
