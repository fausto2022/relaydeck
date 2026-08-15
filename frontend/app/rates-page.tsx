import { RateRanking } from "@/components/monitor/rate-ranking"

export default function RatesPage() {
  return (
    <>
      <h1 className="sr-only">倍率排行</h1>
      <RateRanking standalone />
    </>
  )
}
