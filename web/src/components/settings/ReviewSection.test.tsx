import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import { ApiContext } from "../../api/ApiContext";
import type { BoardApi } from "../../api/boardApi";
import type { BoardSettings } from "../../api/types";
import { ToastProvider } from "../toast/ToastProvider";
import { ReviewSection } from "./ReviewSection";
(globalThis as unknown as {IS_REACT_ACT_ENVIRONMENT: boolean}).IS_REACT_ACT_ENVIRONMENT = true;

const base = {review_by: "other_model", review_fresh_context: true, review_adversarial: true} as BoardSettings;

async function mount(api: Partial<BoardApi>) {
 const host = document.createElement("div"); document.body.append(host);
 const root = createRoot(host);
 await act(async () => root.render(<ApiContext value={api as BoardApi}><ToastProvider><ReviewSection /></ToastProvider></ApiContext>));
 return {host, unmount: async () => {await act(async () => root.unmount()); host.remove();}};
}

it("offers the two standing reviewer choices first, then the accounts, and saves each control", async () => {
 const editSettings = vi.fn().mockImplementation(async (patch) => ({...base, ...patch}));
 const v = await mount({settings: vi.fn().mockResolvedValue(base), editSettings});
 try {
  const select = v.host.querySelector("select")!;
  expect([...select.options].map(o => o.text)).toEqual(["Any model except the engineer's", "Same model as the engineer", "Claude account", "Codex account"]);
  expect(select.value).toBe("other_model");
  await act(async () => {select.value = "same_model"; select.dispatchEvent(new Event("change", {bubbles: true}));});
  expect(editSettings).toHaveBeenLastCalledWith({review_by: "same_model"});
  const [fresh, adversarial] = [...v.host.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')] as [HTMLInputElement, HTMLInputElement];
  expect(fresh.labels?.[0]?.textContent).toContain("Clear of context");
  expect(adversarial.labels?.[0]?.textContent).toContain("Adversarial");
  await act(async () => adversarial.click());
  expect(editSettings).toHaveBeenLastCalledWith({review_adversarial: false});
  expect(adversarial.checked).toBe(false);
 } finally {await v.unmount();}
});

it("restores the setting and shows the refusal", async () => {
 const v = await mount({settings: vi.fn().mockResolvedValue(base), editSettings: vi.fn().mockRejectedValue(new Error("not allowed"))});
 try {
  const fresh = v.host.querySelector<HTMLInputElement>('input[type="checkbox"]')!;
  await act(async () => fresh.click());
  expect(fresh.checked).toBe(true);
  expect(document.body.textContent).toContain("not allowed");
 } finally {await v.unmount();}
});
