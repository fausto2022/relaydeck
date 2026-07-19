import type { NotificationEvent } from "@/lib/api-types"

export interface NotificationEventOption {
  id: string
  label: string
  description: string
  events: NotificationEvent[]
}

export const NOTIFICATION_EVENT_OPTIONS: NotificationEventOption[] = [
  { id: "balance_low", label: "余额不足", description: "余额低于渠道阈值时提醒，并附建议充值金额。", events: ["balance_low"] },
  { id: "rate_changed", label: "倍率变化", description: "已有分组的倍率或补全倍率发生变化。", events: ["rate_changed"] },
  { id: "rate_group_changed", label: "分组变动", description: "上游分组新增、移除或整体结构变化。", events: ["rate_structure_changed", "rate_added", "rate_removed"] },
  { id: "announcement", label: "上游公告", description: "上游站点发布新的公告。", events: ["announcement"] },
  { id: "login_failed", label: "登录失败", description: "渠道凭据失效或登录上游失败。", events: ["login_failed"] },
  { id: "monitor_failed", label: "采集失败", description: "余额、消费、倍率或订阅用量采集失败。", events: ["monitor_failed"] },
  {
    id: "subscription_quota_low",
    label: "订阅额度不足",
    description: "Sub2API 每日、每周或每月剩余额度触发阈值。",
    events: ["subscription_daily_remaining_low", "subscription_weekly_remaining_low", "subscription_monthly_remaining_low"],
  },
  { id: "subscription_expiring", label: "订阅即将到期", description: "Sub2API 订阅进入到期提醒窗口。", events: ["subscription_expiring"] },
  { id: "upstream_sync_group_changed", label: "同步分组变动", description: "同步规则关联的上游分组发生变化。", events: ["upstream_sync_group_changed"] },
  { id: "main_pool_risk", label: "主站分组风险", description: "主站分组进入降级或严重风险状态。", events: ["main_pool_degraded", "main_pool_critical"] },
  { id: "main_margin_risk", label: "主站利润不足", description: "上游成本导致账号利润低于保护线。", events: ["main_member_margin_risk"] },
  { id: "main_margin_recovered", label: "主站利润恢复", description: "账号利润重新满足启用条件。", events: ["main_member_margin_recovered"] },
  { id: "main_member_disabled", label: "主站账号停用", description: "账号因人工设置、健康或利润保护被停用。", events: ["main_member_disabled"] },
  { id: "main_member_reenabled", label: "主站账号恢复", description: "停用账号满足恢复条件后重新启用。", events: ["main_member_reenabled"] },
  { id: "main_member_binding_lost", label: "主站绑定丢失", description: "已接管账号在主站中被删除或失去绑定。", events: ["main_member_binding_lost"] },
  { id: "health_probe_budget_exceeded", label: "探活预算超限", description: "主站账号的每日探活次数或 Token 预算耗尽。", events: ["health_probe_budget_exceeded"] },
]

export const ALL_NOTIFICATION_EVENTS = Array.from(
  new Set(NOTIFICATION_EVENT_OPTIONS.flatMap((option) => option.events)),
)

export const RATE_NOTIFICATION_EVENTS: NotificationEvent[] = [
  "rate_changed",
  "rate_structure_changed",
  "rate_added",
  "rate_removed",
]
