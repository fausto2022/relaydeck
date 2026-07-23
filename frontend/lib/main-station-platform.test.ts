import { describe, expect, it } from "vitest"
import { mainStationHealthAPIMode, normalizeMainStationPlatform } from "./main-station-platform"

describe("main station platform", () => {
  it.each([
    ["anthropic", "anthropic"],
    ["Claude", "anthropic"],
    ["gemini", "gemini"],
    ["Google", "gemini"],
    ["xAI", "grok"],
  ])("normalizes %s", (value, expected) => {
    expect(normalizeMainStationPlatform(value)).toBe(expected)
  })

  it.each([
    ["anthropic", "anthropic"],
    ["claude", "anthropic"],
    ["gemini", "gemini"],
    ["image", "openai_image"],
    ["openai", "openai_chat"],
    ["grok", "openai_chat"],
  ])("uses the correct health API mode for %s", (platform, expected) => {
    expect(mainStationHealthAPIMode(platform)).toBe(expected)
  })
})
