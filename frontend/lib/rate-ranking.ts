import type { RateProviderType, RateRankingConfig, RateSnapshot } from "@/lib/api-types"

export const ALL_RATE_CATEGORY = "__all__"

export interface RateCategoryOption {
  value: string
  label: string
  count: number
}

export function providerRankingRates(rates: RateSnapshot[], provider: RateProviderType) {
  return rates
    .filter((rate) => rate.ranking_visible && rate.ranking_provider === provider)
    .sort((left, right) => left.ratio - right.ratio || left.model_name.localeCompare(right.model_name))
}

export function rateCategoryOptions(
  rates: RateSnapshot[],
  config?: RateRankingConfig | null,
  provider?: RateProviderType,
): RateCategoryOption[] {
  const categories = new Map<string, { count: number; order: number }>()
  if (config && provider) {
    for (const rule of config.rules) {
      if (rule.provider === provider && rule.enabled) {
        categories.set(rule.category_name, { count: 0, order: rule.sort_order })
      }
    }
    const setting = config.providers.find((item) => item.provider === provider)
    if (setting?.include_unmatched ?? true) {
      categories.set("通用", { count: 0, order: 1_000_000 })
    }
  }
  for (const rate of rates) {
    const current = categories.get(rate.ranking_category)
    categories.set(rate.ranking_category, {
      count: (current?.count ?? 0) + 1,
      order: Math.min(current?.order ?? Number.MAX_SAFE_INTEGER, rate.ranking_category_order),
    })
  }
  const options = Array.from(categories.entries())
    .sort((left, right) => left[1].order - right[1].order || left[0].localeCompare(right[0]))
    .map(([label, value]) => ({ value: label, label, count: value.count }))
  return [{ value: ALL_RATE_CATEGORY, label: "全部", count: rates.length }, ...options]
}

export function categoryRankingRates(rates: RateSnapshot[], category: string) {
  if (category === ALL_RATE_CATEGORY) return rates
  return rates.filter((rate) => rate.ranking_category === category)
}
