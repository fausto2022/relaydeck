import { describe, expect, it } from "vitest"
import type { RateSnapshot } from "@/lib/api-types"
import {
  ALL_RATE_CATEGORY,
  categoryRankingRates,
  providerRankingRates,
  rateCategoryOptions,
} from "@/lib/rate-ranking"

function rate(overrides: Partial<RateSnapshot>): RateSnapshot {
  return {
    id: 1,
    channel_id: 1,
    model_name: "OpenAI",
    ratio: 1,
    completion_ratio: 1,
    first_seen_at: "2026-01-01T00:00:00Z",
    last_seen_at: "2026-01-01T00:00:00Z",
    main_station_connected: false,
    main_station_groups: [],
    ranking_provider: "openai",
    ranking_category: "通用",
    ranking_category_order: 1_000_000,
    ranking_visible: true,
    ...overrides,
  }
}

describe("rate ranking categories", () => {
  it("filters hidden rows and sorts by converted ratio", () => {
    const rates = [
      rate({ id: 1, ratio: 0.4 }),
      rate({ id: 2, ratio: 0.2, ranking_category: "Pro", ranking_category_order: 10 }),
      rate({ id: 3, ratio: 0.1, ranking_visible: false }),
      rate({ id: 4, ratio: 0.05, ranking_provider: "anthropic" }),
    ]
    expect(providerRankingRates(rates, "openai").map((item) => item.id)).toEqual([2, 1])
  })

  it("orders custom categories before general and filters selected category", () => {
    const rates = [
      rate({ id: 1 }),
      rate({ id: 2, ranking_category: "Plus", ranking_category_order: 20 }),
      rate({ id: 3, ranking_category: "Pro", ranking_category_order: 10 }),
    ]
    expect(rateCategoryOptions(rates).map((item) => item.value)).toEqual([
      ALL_RATE_CATEGORY,
      "Pro",
      "Plus",
      "通用",
    ])
    expect(categoryRankingRates(rates, "Pro").map((item) => item.id)).toEqual([3])
  })

  it("includes configured categories with no matching rates", () => {
    const options = rateCategoryOptions([], {
      providers: [{ provider: "openai", include_unmatched: false }],
      rules: [{
        provider: "openai",
        category_name: "Pro",
        keywords: ["pro"],
        match_mode: "word",
        sort_order: 10,
        enabled: true,
      }],
    }, "openai")
    expect(options).toEqual([
      { value: ALL_RATE_CATEGORY, label: "全部", count: 0 },
      { value: "Pro", label: "Pro", count: 0 },
    ])
  })
})
