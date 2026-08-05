import { describe, expect, it } from "vitest"
import { isDOMNodeMismatchError } from "@/lib/client-error-reporting"

describe("isDOMNodeMismatchError", () => {
  it("detects React DOM parent-child mismatch errors", () => {
    expect(isDOMNodeMismatchError(new Error("Failed to execute 'removeChild' on 'Node': The node to be removed is not a child of this node."))).toBe(true)
    expect(isDOMNodeMismatchError(new Error("Failed to execute 'insertBefore' on 'Node'"))).toBe(true)
  })

  it("does not classify ordinary application errors as DOM mismatches", () => {
    expect(isDOMNodeMismatchError(new Error("request failed"))).toBe(false)
  })
})
