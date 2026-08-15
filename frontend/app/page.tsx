import { KpiRow } from "@/components/monitor/kpi-row"
import { BalanceOverview } from "@/components/monitor/balance-overview"
import { MultiplierChanges } from "@/components/monitor/multiplier-changes"
import { BottomPanels } from "@/components/monitor/bottom-panels"

export default function Page() {
  return (
    <>
      <h1 className="sr-only">首页看板</h1>
      <KpiRow />

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-12">
        <div className="min-w-0 lg:col-span-7">
          <BalanceOverview />
        </div>
        <div className="min-w-0 lg:col-span-5">
          <MultiplierChanges />
        </div>
      </div>

      <BottomPanels />
    </>
  )
}
