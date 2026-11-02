import { useEffect, useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { AccountUsage } from "../../api/types";

/** Displays cached status; browser polling hits only the board, never an LLM. */
export function UsageBars() {
 const api = useApi();
 const [snapshot, setSnapshot] = useState<AccountUsage>({accounts: []});
 const [now, setNow] = useState(Date.now());
 useEffect(() => {
  let active = true;
  const load = async () => {
   try { const value = await api.accountUsage(); if (active) setSnapshot(value); } catch { /* Keep the last reading, visibly marked stale. */ }
   if (active) setNow(Date.now());
  };
  void load();
  const timer = window.setInterval(() => { if (!document.hidden) void load(); }, 60000);
  return () => { active = false; window.clearInterval(timer); };
 }, [api]);
 return <div className="usage-bars" aria-label="Account usage, percent used">
  {(["codex", "claude"] as const).map((provider) => {
   const account = snapshot.accounts.find((a) => a.provider === provider);
   const window = account?.windows.filter((w) => w.used_percent !== null).reduce<(NonNullable<typeof account>)["windows"][number] | undefined>((best, item) => !best || item.used_percent! > best.used_percent! ? item : best, undefined);
   const value = window?.used_percent;
   const observed = account?.observed_at ? Date.parse(account.observed_at) : NaN;
   const stale = !Number.isFinite(observed) || now - observed > 15 * 60000 || (window?.resets_at != null && Date.parse(window.resets_at) <= now);
   const known = value != null && Number.isFinite(value);
   const used = known ? Math.min(100, Math.max(0, value)) : undefined;
   const label = provider === "codex" ? "Codex" : "Claude";
   const reset = window?.resets_at ? new Date(window.resets_at) : null;
   const resetLabel = reset && Number.isFinite(reset.getTime()) ? `${reset.toLocaleDateString(undefined, {weekday: "short"})} ${reset.getMonth() + 1}/${reset.getDate()}` : null;
   const period = window?.label === "week" ? "weekly" : window?.label;
   const tooltip = `${label}: ${known ? `${used}% used (${window?.label})` : "usage unavailable"}${account?.observed_at ? ` · last read ${new Date(observed).toLocaleString()}` : ""}. Scripted status only; no model tokens. All runners share this account.`;
   return <div key={provider} className={`usage-meter${stale ? " usage-meter--stale" : ""}${used === 100 ? " usage-meter--full" : ""}`} title={tooltip}>
    <div className="usage-meter__label"><span>{label}{period ? ` · ${period}` : ""}{resetLabel ? ` · ${resetLabel}` : ""}</span><span>{known ? `${Math.round(used!)}% used` : "—"}{known && stale ? " · stale" : ""}</span></div>
    <div className="usage-meter__track" role="progressbar" aria-label={`${label} usage`} aria-valuemin={0} aria-valuemax={100} aria-valuenow={used} aria-valuetext={known ? `${used}% used${stale ? ", stale reading" : ""}` : "Usage unavailable"}><span style={{width: `${used ?? 0}%`}} /></div>
   </div>;
  })}
 </div>;
}
