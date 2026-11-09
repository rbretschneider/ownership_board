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
 const api = {registeredRunners:vi.fn().mockResolvedValue({hosts:[]}),settings: vi.fn().mockResolvedValue(initial), editSettings} as unknown as BoardApi;
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
  registeredRunners:vi.fn().mockResolvedValue({hosts:[]}),
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


it("shows host connectivity and enabled online capacity with a settings link",async()=>{
 const api={settings:vi.fn().mockResolvedValue({codex_workers:3,codex_worker_limit:6}),registeredRunners:vi.fn().mockResolvedValue({hosts:[{online:true,enabled:true,max_parallel:3},{online:true,enabled:false,max_parallel:8},{online:false,enabled:true,max_parallel:8}]})} as unknown as BoardApi;
 const host=document.createElement("div"),root=createRoot(host);
 await act(async()=>root.render(<ApiContext value={api}><ToastProvider><AgentWorkSection/></ToastProvider></ApiContext>));
 expect(host.textContent).toContain("2 hosts online · 3 builds at once");
 expect(host.querySelector('a[href="#/settings?section=build-hosts"]')).not.toBeNull();
 expect(host.textContent).not.toContain("Parallel Windows builds");expect(host.querySelector('[role="switch"]')).toBeNull();
 await act(async()=>root.unmount());
});
it("lets the owner turn the legacy Windows switch off and back on while no build host is approved",async()=>{
 const initial={windows_runner_enabled:true,codex_workers:3,codex_worker_limit:6,autonomous_in_flight_cap:3};
 const editSettings=vi.fn().mockResolvedValueOnce({...initial,windows_runner_enabled:false}).mockResolvedValueOnce(initial);
 const api={registeredRunners:vi.fn().mockResolvedValue({hosts:[]}),settings:vi.fn().mockResolvedValue(initial),editSettings} as unknown as BoardApi;
 const host=document.createElement("div"),root=createRoot(host);
 await act(async()=>root.render(<ApiContext value={api}><ToastProvider><AgentWorkSection/></ToastProvider></ApiContext>));
 const toggle=host.querySelector<HTMLInputElement>('[role="switch"]')!;
 expect(toggle.checked).toBe(true);expect(toggle.disabled).toBe(false);
 await act(async()=>toggle.click());
 expect(editSettings).toHaveBeenCalledWith({windows_runner_enabled:false});
 expect(toggle.checked).toBe(false);expect(toggle.disabled).toBe(false);
 await act(async()=>toggle.click());
 expect(editSettings).toHaveBeenLastCalledWith({windows_runner_enabled:true});
 expect(toggle.checked).toBe(true);
 await act(async()=>root.unmount());
});
it("keeps account controls available when hosts fail and retries the host read",async()=>{
 const registeredRunners=vi.fn().mockRejectedValueOnce(new Error("Hosts unavailable")).mockResolvedValue({hosts:[]});
 const api={settings:vi.fn().mockResolvedValue({codex_workers:3,codex_worker_limit:6}),registeredRunners} as unknown as BoardApi;
 const host=document.createElement("div"),root=createRoot(host);
 await act(async()=>root.render(<ApiContext value={api}><ToastProvider><AgentWorkSection/></ToastProvider></ApiContext>));
 expect(host.textContent).toContain("Hosts unavailable");expect(host.querySelector('[aria-label="More Codex workers"]')).not.toBeNull();
 await act(async()=>{Array.from(host.querySelectorAll("button")).find(b=>b.textContent?.includes("Try again"))!.click();});
 expect(host.textContent).toContain("No approved build hosts");
 await act(async()=>root.unmount());
});
it("shows loading, then marks a retained host summary stale after polling fails",async()=>{
 vi.useFakeTimers();
 let resolve:(value:unknown)=>void=()=>{};
 const registeredRunners=vi.fn().mockImplementationOnce(()=>new Promise(done=>{resolve=done;})).mockRejectedValue(new Error("Offline"));
 const api={settings:vi.fn().mockResolvedValue({codex_workers:3,codex_worker_limit:6}),registeredRunners} as unknown as BoardApi;
 const host=document.createElement("div"),root=createRoot(host);
 try {
 await act(async()=>root.render(<ApiContext value={api}><ToastProvider><AgentWorkSection/></ToastProvider></ApiContext>));
 expect(host.textContent).toContain("Loading build hosts");
 await act(async()=>resolve({hosts:[{online:true,enabled:true,max_parallel:1}]}));
 expect(host.textContent).toContain("1 host online · 1 build at once");
 await act(async()=>vi.advanceTimersByTimeAsync(20000));
 expect(host.textContent).toContain("stale");expect(host.textContent).toContain("1 host online");
 }finally{await act(async()=>root.unmount());vi.useRealTimers();}
});
