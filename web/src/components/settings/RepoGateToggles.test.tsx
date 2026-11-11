import { act, useState } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import { ApiContext } from "../../api/ApiContext";
import type { BoardApi } from "../../api/boardApi";
import type { Repo } from "../../api/types";
import { repo as makeRepo } from "../../lib/flowFixture";
import { ToastProvider } from "../toast/ToastProvider";
import { RepoGateToggles } from "./RepoGateToggles";
import { RepositoriesSection } from "./RepositoriesSection";
(globalThis as unknown as {IS_REACT_ACT_ENVIRONMENT: boolean}).IS_REACT_ACT_ENVIRONMENT = true;

async function mount(editRepo: ReturnType<typeof vi.fn>, initial = {...makeRepo(1, "app"), deploy_mode: "off"} as Repo) {
 const host = document.createElement("div"); document.body.append(host);
 const root = createRoot(host);
 function Parent() {
  const [repo, setRepo] = useState(initial);
  return <RepoGateToggles repo={repo} onUpdated={setRepo} />;
 }
 await act(async () => root.render(<ApiContext value={{editRepo} as unknown as BoardApi}><ToastProvider><Parent /></ToastProvider></ApiContext>));
 return {host, select: host.querySelector("select")!, unmount: async () => {await act(async () => root.unmount()); host.remove();}};
}
async function change(select: HTMLSelectElement, value: string) {
 await act(async () => {select.value = value; select.dispatchEvent(new Event("change", {bubbles: true}));});
}
it("labels deployment policy and saves both options with the exact payload", async () => {
 const edit = vi.fn().mockImplementation(async (_id, patch) => ({...makeRepo(1, "app"), ...patch}));
 const v = await mount(edit);
 try {
  expect(v.select).not.toBeNull();
  expect(v.select.labels?.[0]?.textContent).toContain("Deploys");
  expect([...v.select.options].map(o => [o.text, o.value])).toEqual([["Off", "off"], ["Agent deploys", "agent"]]);
  expect(v.host.querySelector(`[id="${v.select.getAttribute("aria-describedby")}"]`)?.textContent).toBe("With Off, merged work goes straight to Deployed.");
  await change(v.select, "agent");
  expect(edit).toHaveBeenLastCalledWith(1, {deploy_mode: "agent"});
  expect(v.select.value).toBe("agent");
  await change(v.select, "off");
  expect(edit).toHaveBeenLastCalledWith(1, {deploy_mode: "off"});
 } finally {await v.unmount();}
});
it("disables checkpoint controls while pending and adopts the server result", async () => {
 let resolve!: (repo: Repo) => void;
 const v = await mount(vi.fn(() => new Promise<Repo>(r => {resolve = r;})));
 try {
  await change(v.select, "agent");
  expect(v.select.value).toBe("agent");
  expect([...v.host.querySelectorAll<HTMLInputElement | HTMLSelectElement>("input,select")].every(e => e.disabled)).toBe(true);
  await act(async () => resolve({...makeRepo(1, "app"), deploy_mode: "off", require_spec_approval: true}));
  expect(v.select.value).toBe("off");
  expect(v.host.querySelector<HTMLInputElement>('input')?.checked).toBe(true);
  expect(v.select.disabled).toBe(false);
 } finally {await v.unmount();}
});
it("restores the policy and shows the refusal", async () => {
 const v = await mount(vi.fn().mockRejectedValue(new Error("policy refused")));
 try {
  await change(v.select, "agent");
  expect(v.select.value).toBe("off");
  expect(document.body.textContent).toContain("policy refused");
 } finally {await v.unmount();}
});
it("persists policy through the repository list and a fresh load", async () => {
 let stored = {...makeRepo(1, "app"), deploy_mode: "off"} as Repo;
 const api = {connections: vi.fn().mockResolvedValue([]), repos: vi.fn(async () => [stored]), editRepo: vi.fn(async (_id, patch) => (stored = {...stored, ...patch}))} as unknown as BoardApi;
 const host = document.createElement("div"); document.body.append(host);
 const render = async () => {
  const root = createRoot(host);
  await act(async () => root.render(<ApiContext value={api}><ToastProvider><RepositoriesSection onChanged={() => {}} /></ToastProvider></ApiContext>));
  return root;
 };
 let root = await render();
 try {
  const select = [...host.querySelectorAll("select")].find(s => s.labels?.[0]?.textContent?.includes("Deploys"))!;
  expect(select).toBeDefined();
  await change(select, "agent");
  await act(async () => root.unmount());
  root = await render();
  expect([...host.querySelectorAll("select")].find(s => s.labels?.[0]?.textContent?.includes("Deploys"))?.value).toBe("agent");
  expect(api.repos).toHaveBeenCalledTimes(2);
 } finally {await act(async () => root.unmount()); host.remove();}
});
it("keeps boolean checkpoint edits working alongside deployment policy", async () => {
 const edit = vi.fn().mockImplementation(async (_id, patch) => ({...makeRepo(1, "app"), ...patch}));
 const v = await mount(edit);
 try {
  await act(async () => v.host.querySelector<HTMLInputElement>('input')!.click());
  expect(edit).toHaveBeenCalledWith(1, {require_spec_approval: true});
  expect(v.host.querySelector<HTMLInputElement>('input')?.checked).toBe(true);
  expect(v.select.value).toBe("off");
 } finally {await v.unmount();}
});
it("shows Off for older repository responses without deployment metadata", async () => {
 const older = {...makeRepo(1, "app")};
 Reflect.deleteProperty(older, "deploy_mode");
 const v = await mount(vi.fn(), older);
 try {expect(v.select.value).toBe("off");} finally {await v.unmount();}
});
it("saves which review findings block publication in the repo", async () => {
 const edit = vi.fn().mockImplementation(async (_id, patch) => ({...makeRepo(1, "app"), ...patch}));
 const v = await mount(edit);
 try {
  const select = v.host.querySelectorAll("select")[1] as HTMLSelectElement;
  expect(select.labels?.[0]?.textContent).toContain("Review blocks on");
  expect([...select.options].map(o => o.value)).toEqual(["p0", "p1", "p2", "p3"]);
  expect(select.value).toBe("p1");
  await change(select, "p3");
  expect(edit).toHaveBeenLastCalledWith(1, {review_blocking: "p3"});
  expect(select.value).toBe("p3");
 } finally {await v.unmount();}
});
