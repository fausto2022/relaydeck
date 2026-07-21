import { describe, expect, it } from "vitest"
import { foregroundRefreshIsDue } from "@/lib/refresh-context"

describe("foreground refresh deduplication", () => {
  it("refreshes immediately the first time the page returns to the foreground", () => {
    expect(foregroundRefreshIsDue(null, 10_000)).toBe(true)
  })

  it("coalesces visibility and focus events from the same foreground transition", () => {
    expect(foregroundRefreshIsDue(10_000, 10_999)).toBe(false)
    expect(foregroundRefreshIsDue(10_000, 11_000)).toBe(true)
  })

  it("refreshes when the system clock moves backwards", () => {
    expect(foregroundRefreshIsDue(10_000, 9_000)).toBe(true)
  })
})
