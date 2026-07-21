import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import type { RateSnapshot } from "@/lib/api-types"

const apiFetchMock = vi.hoisted(() => vi.fn())

vi.mock("@/lib/api", () => ({ apiFetch: apiFetchMock }))

import { fetchShared, mergeSettledChannelRates } from "@/lib/queries"

function rate(id: number, channelID: number, ratio: number): RateSnapshot {
  return {
    id,
    channel_id: channelID,
    model_name: `group-${id}`,
    ratio,
    completion_ratio: 1,
    first_seen_at: "2026-01-01T00:00:00Z",
    last_seen_at: "2026-01-01T00:00:00Z",
    main_station_connected: false,
    main_station_groups: [],
    ranking_provider: "openai",
    ranking_category: "通用",
    ranking_category_order: 1_000_000,
    ranking_visible: true,
  }
}

describe("fetchShared", () => {
  beforeEach(() => {
    vi.useFakeTimers()
    apiFetchMock.mockReset()
  })

  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
  })

  it("bypasses the browser cache and only shares a response during the short TTL", async () => {
    apiFetchMock.mockResolvedValue({ value: 1 })

    await expect(fetchShared("/test-rates", "test-rates#1")).resolves.toEqual({ value: 1 })
    await expect(fetchShared("/test-rates", "test-rates#1")).resolves.toEqual({ value: 1 })

    expect(apiFetchMock).toHaveBeenCalledTimes(1)
    expect(apiFetchMock).toHaveBeenCalledWith("/test-rates", { cache: "no-store" })

    await vi.advanceTimersByTimeAsync(800)
    await fetchShared("/test-rates", "test-rates#1")

    expect(apiFetchMock).toHaveBeenCalledTimes(2)
  })
})

describe("mergeSettledChannelRates", () => {
  it("updates successful channels and retains the previous rows for failed channels", () => {
    const previous = [rate(1, 1, 0.8), rate(2, 2, 0.9), rate(3, 3, 1)]
    const results: PromiseSettledResult<RateSnapshot[]>[] = [
      { status: "fulfilled", value: [rate(4, 1, 0.5)] },
      { status: "rejected", reason: new Error("temporary failure") },
    ]

    const merged = mergeSettledChannelRates(previous, [1, 2], results)

    expect(merged?.map((item) => [item.id, item.channel_id, item.ratio])).toEqual([
      [4, 1, 0.5],
      [2, 2, 0.9],
    ])
  })

  it("keeps null when every channel fails before the first successful load", () => {
    const results: PromiseSettledResult<RateSnapshot[]>[] = [
      { status: "rejected", reason: new Error("temporary failure") },
    ]

    expect(mergeSettledChannelRates(null, [1], results)).toBeNull()
  })
})
