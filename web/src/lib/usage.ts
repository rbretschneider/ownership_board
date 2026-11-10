import type { TokenUsage } from "../api/types";

/**
 * Weighted tokens above which a ticket is flagged in bold red. A tightly
 * scoped story that one Sonnet-sized pass can finish uses roughly 300k–500k;
 * this is several times that, so only the runaway ones stand out.
 */
export const HEAVY_TOKENS = 3_000_000;

/** 1234 → "1.2k", 3_400_000 → "3.4M". */
export function formatTokens(n: number): string {
  if (n < 1000) return String(n);
  if (n < 1_000_000) return `${trim(n / 1000)}k`;
  return `${trim(n / 1_000_000)}M`;
}

function trim(value: number): string {
  return value >= 100 ? value.toFixed(0) : value.toFixed(1).replace(/\.0$/, "");
}

export interface UsageBadge {
  label: string;
  title: string;
  heavy: boolean;
}

/** What a card shows for a ticket's reported usage; undefined when none was reported. */
export function usageBadge(usage: TokenUsage | undefined): UsageBadge | undefined {
  if (!usage || usage.calls === 0) return undefined;
  const fresh = usage.input_tokens + usage.cache_write_tokens + usage.output_tokens;
  const parts = [
    `${formatTokens(usage.weighted_tokens)} tokens over ${usage.calls} model ${usage.calls === 1 ? "call" : "calls"}`,
    `${formatTokens(fresh)} new input and output`,
  ];
  if (usage.cache_read_tokens > 0) parts.push(`${formatTokens(usage.cache_read_tokens)} cache reads, counted at a tenth`);
  if (usage.cost_usd > 0) parts.push(`Claude list price $${usage.cost_usd.toFixed(2)}`);
  const heavy = usage.weighted_tokens >= HEAVY_TOKENS;
  if (heavy) parts.push(`over ${formatTokens(HEAVY_TOKENS)}, far more than a tightly scoped story`);
  return { label: `${formatTokens(usage.weighted_tokens)} tok`, title: parts.join(" · "), heavy };
}
