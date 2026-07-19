import { describe, expect, it } from "vitest"
import { formatRatio, ratioDelta, relativeTime } from "./format"

describe("format helpers", () => {
  it("formats recent timestamps deterministically", () => {
    const now = new Date("2026-07-19T12:00:00Z")
    expect(relativeTime("2026-07-19T11:58:30Z", now)).toBe("1 分钟前")
  })

  it("formats ratio changes", () => {
    expect(formatRatio(1.25)).toBe("1.25")
    expect(ratioDelta(1, 1.25)).toEqual({ direction: "up", pct: "+25.0%" })
  })
})
