import { getToken } from "@/lib/api"

const DOM_RECOVERY_KEY = "relaydeck_dom_recovery_at"
const DOM_RECOVERY_COOLDOWN_MS = 10 * 60 * 1000

interface ClientErrorReport {
  name: string
  message: string
  stack: string
  component_stack: string
  url: string
  user_agent: string
  document_language: string
  dom_mismatch: boolean
  translation_detected: boolean
}

export function isDOMNodeMismatchError(error: unknown) {
  const message = error instanceof Error ? error.message : String(error ?? "")
  return message.includes("removeChild") || message.includes("insertBefore")
}

export function reportClientError(error: Error, componentStack: string) {
  if (typeof window === "undefined") return

  try {
    const payload: ClientErrorReport = {
      name: truncate(error.name, 128),
      message: truncate(error.message, 2_000),
      stack: truncate(error.stack ?? "", 16_000),
      component_stack: truncate(componentStack, 16_000),
      url: truncate(`${window.location.origin}${window.location.pathname}`, 2_000),
      user_agent: truncate(window.navigator.userAgent, 1_000),
      document_language: truncate(document.documentElement.lang, 32),
      dom_mismatch: isDOMNodeMismatchError(error),
      translation_detected: browserTranslationDetected(),
    }
    const headers = new Headers({
      Accept: "application/json",
      "Content-Type": "application/json",
    })
    const token = getToken()
    if (token) headers.set("Authorization", `Bearer ${token}`)
    void fetch("/api/client-errors", {
      method: "POST",
      headers,
      body: JSON.stringify(payload),
      keepalive: true,
    }).catch((reportError: unknown) => {
      console.warn("client error report failed", reportError)
    })
  } catch (reportError) {
    console.warn("client error report failed", reportError)
  }
}

export function scheduleDOMMismatchRecovery(error: unknown) {
  if (typeof window === "undefined" || !isDOMNodeMismatchError(error)) return false

  try {
    const now = Date.now()
    const previous = Number(window.sessionStorage.getItem(DOM_RECOVERY_KEY) ?? 0)
    if (Number.isFinite(previous) && now - previous < DOM_RECOVERY_COOLDOWN_MS) return false
    window.sessionStorage.setItem(DOM_RECOVERY_KEY, String(now))
  } catch (storageError) {
    console.warn("DOM mismatch recovery state unavailable", storageError)
    return false
  }
  window.setTimeout(() => window.location.reload(), 150)
  return true
}

function browserTranslationDetected() {
  const root = document.documentElement
  return root.classList.contains("translated-ltr") ||
    root.classList.contains("translated-rtl") ||
    document.querySelector("font[style*='vertical-align']") !== null
}

function truncate(value: string, limit: number) {
  const text = String(value ?? "")
  return text.length > limit ? text.slice(0, limit) : text
}
