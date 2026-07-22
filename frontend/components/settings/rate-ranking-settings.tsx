"use client"

import { useEffect, useRef, useState } from "react"
import { ArrowDown, ArrowUp, PencilLine, Plus, Tags, Trash2 } from "lucide-react"
import { toast } from "sonner"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { apiFetch } from "@/lib/api"
import type { RateProviderType, RateRankingConfig, RateRankingRule } from "@/lib/api-types"
import { useRateRankingConfig } from "@/lib/queries"
import { cn } from "@/lib/utils"

const PROVIDERS: Array<{ value: RateProviderType; label: string }> = [
  { value: "openai", label: "OpenAI" },
  { value: "anthropic", label: "Anthropic" },
  { value: "gemini", label: "Gemini" },
  { value: "antigravity", label: "Antigravity" },
  { value: "grok", label: "Grok" },
]

type EditableRule = RateRankingRule & { clientKey: string }
type EditableConfig = Omit<RateRankingConfig, "rules"> & { rules: EditableRule[] }

interface RuleForm {
  provider: RateProviderType
  category_name: string
  keywords: string
  match_mode: RateRankingRule["match_mode"]
  sort_order: number
  enabled: boolean
}

export function RateRankingSettings() {
  const query = useRateRankingConfig()
  const keyCounter = useRef(0)
  const [config, setConfig] = useState<EditableConfig | null>(null)
  const [selectedProvider, setSelectedProvider] = useState<RateProviderType>("openai")
  const [saving, setSaving] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingKey, setEditingKey] = useState<string | null>(null)
  const [form, setForm] = useState<RuleForm>(() => emptyForm("openai", 10))

  useEffect(() => {
    if (!query.data) return
    setConfig({
      providers: query.data.providers,
      rules: query.data.rules.map((rule) => ({ ...rule, clientKey: String(rule.id ?? ++keyCounter.current) })),
    })
  }, [query.data])

  if (query.loading && !config) return <div className="text-sm text-muted-foreground">加载倍率分类配置中...</div>
  if (query.error && !config) return <div className="text-sm text-destructive">{query.error}</div>
  if (!config) return null

  const currentConfig = config
  const editableRules = currentConfig.rules
  const providerSetting = currentConfig.providers.find((item) => item.provider === selectedProvider)
  const providerRules = editableRules
    .filter((rule) => rule.provider === selectedProvider)
    .sort((left, right) => left.sort_order - right.sort_order || (left.id ?? 0) - (right.id ?? 0))

  function updateConfig(next: EditableConfig) {
    setConfig(next)
  }

  function openNewRule() {
    const maxOrder = providerRules.reduce((max, rule) => Math.max(max, rule.sort_order), 0)
    setEditingKey(null)
    setForm(emptyForm(selectedProvider, maxOrder + 10))
    setDialogOpen(true)
  }

  function openEditRule(rule: EditableRule) {
    setEditingKey(rule.clientKey)
    setForm({
      provider: rule.provider,
      category_name: rule.category_name,
      keywords: rule.keywords.join("\n"),
      match_mode: rule.match_mode,
      sort_order: rule.sort_order,
      enabled: rule.enabled,
    })
    setDialogOpen(true)
  }

  function submitRule() {
    const categoryName = form.category_name.trim()
    const keywords = form.keywords.split(/[,，\n]/).map((item) => item.trim()).filter(Boolean)
    if (!categoryName || keywords.length === 0) {
      toast.error("请填写分类名称和至少一个关键词")
      return
    }
    const duplicate = editableRules.some((rule) =>
      rule.clientKey !== editingKey && rule.provider === form.provider && rule.category_name.trim().toLowerCase() === categoryName.toLowerCase(),
    )
    if (duplicate) {
      toast.error("同一个类型下不能有重复的分类名称")
      return
    }
    const existing = editingKey ? editableRules.find((rule) => rule.clientKey === editingKey) : undefined
    const nextRule: EditableRule = {
      id: existing?.id,
      clientKey: existing?.clientKey ?? `new-${++keyCounter.current}`,
      provider: form.provider,
      category_name: categoryName,
      keywords,
      match_mode: form.match_mode,
      sort_order: form.sort_order > 0 ? form.sort_order : 10,
      enabled: form.enabled,
    }
    const nextRules = editingKey
      ? editableRules.map((rule) => rule.clientKey === editingKey ? nextRule : rule)
      : [...editableRules, nextRule]
    updateConfig({ ...currentConfig, rules: nextRules })
    setSelectedProvider(form.provider)
    setDialogOpen(false)
  }

  function toggleFallback(enabled: boolean) {
    updateConfig({
      ...currentConfig,
      providers: currentConfig.providers.map((item) => item.provider === selectedProvider ? { ...item, include_unmatched: enabled } : item),
    })
  }

  function toggleRule(rule: EditableRule, enabled: boolean) {
    updateConfig({ ...currentConfig, rules: editableRules.map((item) => item.clientKey === rule.clientKey ? { ...item, enabled } : item) })
  }

  function deleteRule(rule: EditableRule) {
    updateConfig({ ...currentConfig, rules: editableRules.filter((item) => item.clientKey !== rule.clientKey) })
  }

  function moveRule(rule: EditableRule, direction: -1 | 1) {
    const index = providerRules.findIndex((item) => item.clientKey === rule.clientKey)
    const target = index + direction
    if (index < 0 || target < 0 || target >= providerRules.length) return
    const reordered = [...providerRules]
    ;[reordered[index], reordered[target]] = [reordered[target], reordered[index]]
    const orderMap = new Map(reordered.map((item, itemIndex) => [item.clientKey, (itemIndex + 1) * 10]))
    updateConfig({
      ...currentConfig,
      rules: editableRules.map((item) => orderMap.has(item.clientKey) ? { ...item, sort_order: orderMap.get(item.clientKey) ?? item.sort_order } : item),
    })
  }

  async function save() {
    setSaving(true)
    try {
      const payload: RateRankingConfig = {
        providers: currentConfig.providers,
        rules: editableRules.map((rule) => ({
          id: rule.id,
          provider: rule.provider,
          category_name: rule.category_name,
          keywords: rule.keywords,
          match_mode: rule.match_mode,
          sort_order: rule.sort_order,
          enabled: rule.enabled,
        })),
      }
      const saved = await apiFetch<RateRankingConfig>("/settings/rate-ranking", { method: "PUT", body: JSON.stringify(payload) })
      setConfig({
        providers: saved.providers,
        rules: saved.rules.map((rule) => ({ ...rule, clientKey: String(rule.id ?? ++keyCounter.current) })),
      })
      toast.success("倍率排行分类配置已保存")
      query.refetch()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "保存分类配置失败")
    } finally {
      setSaving(false)
    }
  }

  return (
    <section className="space-y-5 rounded-3xl border border-border/80 bg-muted/20 p-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-1.5">
          <div className="flex items-center gap-2 text-sm font-semibold text-foreground"><Tags className="size-4 text-sky-600" />倍率排行分类</div>
          <p className="max-w-3xl text-sm leading-6 text-muted-foreground">先按上游分组类型归入主分类，再按分组名称匹配关键词，忽略大小写。例如可在 OpenAI 下新增“生图”分类。</p>
        </div>
        <Button size="sm" variant="outline" onClick={openNewRule}><Plus className="size-3.5" />新增分类</Button>
      </div>

      <Tabs value={selectedProvider} onValueChange={(value) => setSelectedProvider(value as RateProviderType)}>
        <TabsList className="h-auto max-w-full justify-start overflow-x-auto">
          {PROVIDERS.map((provider) => <TabsTrigger key={provider.value} value={provider.value} className="px-3">{provider.label}</TabsTrigger>)}
        </TabsList>
      </Tabs>

      <div className="flex items-start justify-between gap-4 rounded-2xl border border-border bg-background/90 px-4 py-3">
        <div className="space-y-1">
          <Label htmlFor="rate-ranking-include-unmatched" className="text-sm font-medium">未命中归入“通用”</Label>
          <p className="text-[11px] leading-5 text-muted-foreground">关闭后，本类型没有命中自定义规则的分组不会出现在首页排行和查看更多中。</p>
        </div>
        <Switch id="rate-ranking-include-unmatched" checked={providerSetting?.include_unmatched ?? true} onCheckedChange={toggleFallback} />
      </div>

      {providerRules.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-border px-5 py-10 text-center text-sm text-muted-foreground">当前类型还没有自定义分类，所有分组会归入“通用”。</div>
      ) : (
        <div className="space-y-3">
          {providerRules.map((rule, index) => (
            <div key={rule.clientKey} className={cn("rounded-2xl border border-border bg-background/90 p-4", !rule.enabled && "opacity-60")}>
              <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                <div className="min-w-0 space-y-2">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="outline" className="tabular-nums">优先级 {index + 1}</Badge>
                    <span className="text-sm font-semibold">{rule.category_name}</span>
                    <Badge variant="outline">{rule.match_mode === "word" ? "完整词" : "包含"}</Badge>
                    <Badge variant="outline" className={rule.enabled ? "text-emerald-700" : "text-muted-foreground"}>{rule.enabled ? "启用" : "停用"}</Badge>
                  </div>
                  <div className="flex flex-wrap gap-1.5">
                    {rule.keywords.map((keyword) => <Badge key={`${rule.clientKey}-${keyword}`} variant="secondary" className="font-normal">{keyword}</Badge>)}
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  <Button size="icon-sm" variant="ghost" title="上移" aria-label="上移" disabled={index === 0} onClick={() => moveRule(rule, -1)}><ArrowUp /></Button>
                  <Button size="icon-sm" variant="ghost" title="下移" aria-label="下移" disabled={index === providerRules.length - 1} onClick={() => moveRule(rule, 1)}><ArrowDown /></Button>
                  <Switch checked={rule.enabled} onCheckedChange={(checked) => toggleRule(rule, checked)} aria-label={`${rule.category_name}启用状态`} />
                  <Button size="icon-sm" variant="ghost" title="编辑" aria-label="编辑" onClick={() => openEditRule(rule)}><PencilLine /></Button>
                  <Button size="icon-sm" variant="ghost" title="删除" aria-label="删除" className="text-destructive hover:bg-destructive/10" onClick={() => deleteRule(rule)}><Trash2 /></Button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      <div className="flex flex-wrap items-center gap-3 border-t border-border pt-5">
        <Button onClick={() => void save()} disabled={saving}>{saving ? "保存中..." : "保存分类配置"}</Button>
        <span className="text-xs text-muted-foreground">未命中开关和分类规则保存后立即影响排行榜展示。</span>
      </div>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{editingKey ? "编辑倍率分类" : "新增倍率分类"}</DialogTitle>
            <DialogDescription>只匹配上游分组名称，不匹配模型描述。关键词之间使用换行或逗号分隔。</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            <div className="space-y-2"><Label>所属类型</Label><Select value={form.provider} onValueChange={(value) => setForm((current) => ({ ...current, provider: value as RateProviderType }))}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{PROVIDERS.map((provider) => <SelectItem key={provider.value} value={provider.value}>{provider.label}</SelectItem>)}</SelectContent></Select></div>
            <div className="space-y-2"><Label htmlFor="rate-category-name">分类名称</Label><Input id="rate-category-name" value={form.category_name} onChange={(event) => setForm((current) => ({ ...current, category_name: event.target.value }))} placeholder="例如：Pro" /></div>
            <div className="space-y-2"><Label htmlFor="rate-category-keywords">关键词</Label><Textarea id="rate-category-keywords" value={form.keywords} onChange={(event) => setForm((current) => ({ ...current, keywords: event.target.value }))} placeholder="pro\n专业版" rows={4} /></div>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-2"><Label>匹配方式</Label><Select value={form.match_mode} onValueChange={(value) => setForm((current) => ({ ...current, match_mode: value as RuleForm["match_mode"] }))}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent><SelectItem value="contains">包含匹配</SelectItem><SelectItem value="word">完整词匹配</SelectItem></SelectContent></Select></div>
              <div className="space-y-2"><Label htmlFor="rate-category-order">优先级顺序</Label><Input id="rate-category-order" type="number" min={1} value={form.sort_order} onChange={(event) => setForm((current) => ({ ...current, sort_order: Number(event.target.value) }))} /></div>
            </div>
            <div className="flex items-center justify-between rounded-xl border border-border px-3 py-2"><Label htmlFor="rate-category-enabled">启用规则</Label><Switch id="rate-category-enabled" checked={form.enabled} onCheckedChange={(enabled) => setForm((current) => ({ ...current, enabled }))} /></div>
          </div>
          <DialogFooter><Button variant="outline" onClick={() => setDialogOpen(false)}>取消</Button><Button onClick={submitRule}>确定</Button></DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  )
}

function emptyForm(provider: RateProviderType, sortOrder: number): RuleForm {
  return { provider, category_name: "", keywords: "", match_mode: "contains", sort_order: sortOrder, enabled: true }
}
