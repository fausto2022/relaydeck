import { describe, expect, it } from "vitest"
import { isBalanceLow } from "./channel-balance"

describe("channel balance helpers", () => {
  it("marks a sampled balance below a positive threshold as low", () => {
    expect(isBalanceLow(4.99, 5)).toBe(true)
    expect(isBalanceLow(5, 5)).toBe(false)
  })

  it("does not mark missing balances or disabled thresholds as low", () => {
    expect(isBalanceLow(null, 5)).toBe(false)
    expect(isBalanceLow(1, 0)).toBe(false)
    expect(isBalanceLow(1, null)).toBe(false)
  })
})
