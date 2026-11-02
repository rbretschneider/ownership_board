import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import { ApiContext } from "../../api/ApiContext";
import type { BoardApi } from "../../api/boardApi";
import { UsageBars } from "./UsageBars";

(globalThis as unknown as {IS_REACT_ACT_ENVIRONMENT: boolean}).IS_REACT_ACT_ENVIRONMENT = true;
it("fills upward with percent used, and keeps unknown usage distinct from zero", async () => {
 const accountUsage = vi.fn().mockResolvedValue({accounts: [{provider: "codex", observed_at: new Date().toISOString(), windows: [{label: "week", used_percent: 13, resets_at: "2099-01-01T00:00:00Z"}]}]});
 const api = {accountUsage} as unknown as BoardApi;
 const host = document.createElement("div");
 const root = createRoot(host);
 await act(async () => { root.render(<ApiContext value={api}><UsageBars /></ApiContext>); });
 const bars = host.querySelectorAll('[role="progressbar"]');
 expect(bars).toHaveLength(2);
 expect(bars[0]?.getAttribute("aria-valuenow")).toBe("13");
 expect((bars[0]?.firstElementChild as HTMLElement).style.width).toBe("13%");
 expect(host.textContent).toContain("13% used");
 expect(bars[1]?.hasAttribute("aria-valuenow")).toBe(false);
 expect(bars[1]?.getAttribute("aria-valuetext")).toBe("Usage unavailable");
 expect(accountUsage).toHaveBeenCalledTimes(1);
 await act(async () => { root.unmount(); });
});
