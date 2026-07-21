"use client"

import {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
  useRef,
  type ReactNode,
} from "react"

interface RefreshContextValue {
  tick: number
  bump: () => void
}

const RefreshContext = createContext<RefreshContextValue>({
  tick: 0,
  bump: () => {},
})

/** 全局后台轮询周期；后端 cron 是分钟级，这里 30s 已足够"显得活着"。 */
const POLL_INTERVAL_MS = 30_000
const FOREGROUND_REFRESH_DEDUP_MS = 1_000

export function foregroundRefreshIsDue(
  lastRefreshAt: number | null,
  now: number,
) {
  return (
    lastRefreshAt === null ||
    now < lastRefreshAt ||
    now - lastRefreshAt >= FOREGROUND_REFRESH_DEDUP_MS
  )
}

export function RefreshProvider({ children }: { children: ReactNode }) {
  const [tick, setTick] = useState(0)
  const lastForegroundRefreshAt = useRef<number | null>(null)
  const bump = useCallback(() => setTick((t) => t + 1), [])

  // 30 秒静默 polling。页面在后台标签时（document.hidden）不轮询，避免后台浪费请求。
  useEffect(() => {
    const id = setInterval(() => {
      if (typeof document !== "undefined" && document.hidden) return
      setTick((t) => t + 1)
    }, POLL_INTERVAL_MS)
    return () => clearInterval(id)
  }, [])

  // 后台标签恢复或窗口重新聚焦时立即刷新；浏览器通常会连续派发两个事件，需要合并。
  useEffect(() => {
    const refreshWhenForegrounded = () => {
      if (document.hidden) return

      const now = Date.now()
      if (!foregroundRefreshIsDue(lastForegroundRefreshAt.current, now)) return

      lastForegroundRefreshAt.current = now
      setTick((t) => t + 1)
    }

    document.addEventListener("visibilitychange", refreshWhenForegrounded)
    window.addEventListener("focus", refreshWhenForegrounded)
    return () => {
      document.removeEventListener("visibilitychange", refreshWhenForegrounded)
      window.removeEventListener("focus", refreshWhenForegrounded)
    }
  }, [])

  return (
    <RefreshContext.Provider value={{ tick, bump }}>
      {children}
    </RefreshContext.Provider>
  )
}

/** useRefreshTick 在 tick 变化时让组件重新拉数据。 */
export function useRefreshTick() {
  return useContext(RefreshContext).tick
}

/** useTriggerRefresh 返回手动 bump 的方法，比如点头部的"刷新"按钮。 */
export function useTriggerRefresh() {
  return useContext(RefreshContext).bump
}
