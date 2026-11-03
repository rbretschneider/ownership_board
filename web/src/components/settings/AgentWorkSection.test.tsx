import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import { ApiContext } from "../../api/ApiContext";
import type { BoardApi } from "../../api/boardApi";
import { ToastProvider } from "../toast/ToastProvider";
import { AgentWorkSection } from "./AgentWorkSection";

(globalThis as unknown as {IS_REACT_ACT_ENVIRONMENT: boolean}).IS_REACT_ACT_ENVIRONMENT = true;
it("keeps rapid + clicks and serializes saves to the final desired count", async () => {
 const initial = {windows_runner_enabled: false, codex_workers: 3, codex_worker_limit: 6, autonomous_in_flight_cap: 3};
 let resolve: (value: typeof initial) => void = () => {};
 const editSettings = vi.fn(() => new Promise<typeof initial>((done) => { resolve = done; }));
 const api = {settings: vi.fn().mockResolvedValue(initial), editSettings} as unknown as BoardApi;
 const host = document.createElement("div");
 const root = createRoot(host);
 await act(async () => { root.render(<ApiContext value={api}><ToastProvider><AgentWorkSection /></ToastProvider></ApiContext>); });
 const plus = host.querySelector<HTMLButtonElement>('[aria-label="More Codex workers"]')!;
 await act(async () => { plus.click(); plus.click(); plus.click(); });
 expect(host.querySelector<HTMLInputElement>('input[id$="-codex-workers"]')?.value).toBe("6");
 expect(editSettings).toHaveBeenCalledTimes(1);
 await act(async () => { resolve({...initial, codex_workers: 4, autonomous_in_flight_cap: 4}); });
 expect(editSettings).toHaveBeenLastCalledWith({codex_workers: 6, autonomous_in_flight_cap: 6});
 await act(async () => { resolve({...initial, codex_workers: 6, autonomous_in_flight_cap: 6}); });
 expect(plus.disabled).toBe(true);
 expect(host.textContent).toContain("Up to 6 working");
 await act(async () => root.unmount());
});

it("reads back the actual count when a committed save loses its response", async () => {
 const initial = {windows_runner_enabled: false, codex_workers: 3, codex_worker_limit: 6, autonomous_in_flight_cap: 3};
 const api = {
  settings: vi.fn().mockResolvedValueOnce(initial).mockResolvedValue({...initial, codex_workers: 4}),
  editSettings: vi.fn().mockRejectedValue(new Error("Response lost")),
 } as unknown as BoardApi;
 const host = document.createElement("div");
 const root = createRoot(host);
 await act(async () => root.render(<ApiContext value={api}><ToastProvider><AgentWorkSection /></ToastProvider></ApiContext>));
 await act(async () => host.querySelector<HTMLButtonElement>('[aria-label="More Codex workers"]')!.click());
 expect(host.querySelector<HTMLInputElement>('input[id$="-codex-workers"]')?.value).toBe("4");
 expect(api.settings).toHaveBeenCalledTimes(2);
 await act(async () => root.unmount());
});

it("lets the owner enable Windows verification with a separate global switch", async () => {
 const initial = {windows_runner_enabled: false, codex_workers: 3, codex_worker_limit: 6, autonomous_in_flight_cap: 3};
 const editSettings=vi.fn().mockResolvedValue({...initial, windows_runner_enabled:true});
 const api={settings:vi.fn().mockResolvedValue(initial),editSettings} as unknown as BoardApi;
 const host=document.createElement("div"); const root=createRoot(host);
 await act(async()=>root.render(<ApiContext value={api}><ToastProvider><AgentWorkSection /></ToastProvider></ApiContext>));
 const toggle=host.querySelector<HTMLInputElement>('[role="switch"]')!;
 expect(toggle.checked).toBe(false);
 await act(async()=>toggle.click());
 expect(editSettings).toHaveBeenCalledWith({windows_runner_enabled:true});
 expect(toggle.checked).toBe(true);
 await act(async()=>root.unmount());
});

it("serializes rapid Windows scaling without changing the global switch",async()=>{
 const initial={windows_runner_enabled:true,windows_parallel_builds:3,windows_parallel_build_limit:6,codex_workers:4,codex_worker_limit:6,claude_workers:0,claude_worker_limit:6,codex_weekly_limit:100,claude_weekly_limit:100,autonomous_in_flight_cap:4};
 let resolve:(value:typeof initial)=>void=()=>{};
 const editSettings=vi.fn(()=>new Promise<typeof initial>(done=>{resolve=done;}));
 const api={settings:vi.fn().mockResolvedValue(initial),editSettings} as unknown as BoardApi;
 const host=document.createElement("div"),root=createRoot(host);
 await act(async()=>root.render(<ApiContext value={api}><ToastProvider><AgentWorkSection/></ToastProvider></ApiContext>));
 const plus=host.querySelector<HTMLButtonElement>('[aria-label="More Windows builds"]')!;
 await act(async()=>{plus.click();plus.click();plus.click();});
 expect(host.querySelector<HTMLInputElement>('input[id$="-windows-builds"]')?.value).toBe("6");
 expect(editSettings).toHaveBeenCalledWith({windows_parallel_builds:4});
 await act(async()=>resolve({...initial,windows_parallel_builds:4}));
 expect(editSettings).toHaveBeenLastCalledWith({windows_parallel_builds:6});
 await act(async()=>resolve({...initial,windows_parallel_builds:6}));
 expect(host.querySelector<HTMLInputElement>('[role="switch"]')?.checked).toBe(true);
 await act(async()=>root.unmount());
});
