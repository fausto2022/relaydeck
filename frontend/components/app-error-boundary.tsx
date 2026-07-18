import { Component, type ErrorInfo, type ReactNode } from "react"
import { AlertTriangle, RefreshCw } from "lucide-react"
import { Button } from "@/components/ui/button"

interface Props {
  children: ReactNode
}

interface State {
  error: Error | null
}

export class AppErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("application render failed", error, info)
  }

  render() {
    if (!this.state.error) return this.props.children

    return (
      <main className="flex min-h-screen items-center justify-center bg-background px-4">
        <section className="w-full max-w-lg border-y py-8 text-center">
          <AlertTriangle className="mx-auto size-8 text-destructive" />
          <h1 className="mt-4 text-lg font-semibold">页面加载失败</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            {this.state.error.message || "页面组件发生异常，请重新加载。"}
          </p>
          <Button className="mt-5" onClick={() => window.location.reload()}>
            <RefreshCw className="size-4" />重新加载
          </Button>
        </section>
      </main>
    )
  }
}
