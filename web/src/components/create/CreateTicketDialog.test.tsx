import { act } from "react";
import { createRoot } from "react-dom/client";
import { afterEach, expect, it, vi } from "vitest";
import { ApiContext } from "../../api/ApiContext";
import type { BoardApi } from "../../api/boardApi";
import type { Repo } from "../../api/types";
import { encodeRepoFilter } from "../../lib/repos";
import { preferences, STORAGE_KEYS } from "../../lib/storage";
import { CreateTicketDialog } from "./CreateTicketDialog";

(globalThis as unknown as {IS_REACT_ACT_ENVIRONMENT: boolean}).IS_REACT_ACT_ENVIRONMENT = true;

const REPO = {id: 7, full_name: "acme/api", name: "api", is_archived: false} as unknown as Repo;

async function open(lastRepo: "repo" | "none") {
 preferences.write(STORAGE_KEYS.lastRepo, encodeRepoFilter(lastRepo === "repo" ? {kind: "repo", id: REPO.id} : {kind: "none"}));
 const createTicket = vi.fn().mockResolvedValue({ticket: {id: 1}});
 const api = {createTicket} as unknown as BoardApi;
 const host = document.createElement("div");
 document.body.append(host);
 const root = createRoot(host);
 await act(async () => { root.render(<ApiContext value={api}><CreateTicketDialog repos={[REPO]} onClose={() => {}} onCreated={() => {}} /></ApiContext>); });
 const toggle = document.getElementById("create-autonomous") as HTMLInputElement;
 const title = document.getElementById("create-title") as HTMLInputElement;
 const typeTitle = async (text: string) => {
  await act(async () => {
   Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set?.call(title, text);
   title.dispatchEvent(new Event("input", {bubbles: true}));
  });
 };
 const submit = async () => {
  await act(async () => { (document.querySelector('button[type="submit"]') as HTMLButtonElement).click(); });
 };
 return {createTicket, toggle, typeTitle, submit, close: () => act(async () => { root.unmount(); host.remove(); })};
}

afterEach(() => { preferences.write(STORAGE_KEYS.lastRepo, "none"); });

it("creates a ticket on autopilot when the switch is on", async () => {
 const dialog = await open("repo");
 expect(dialog.toggle.disabled).toBe(false);
 expect(dialog.toggle.checked).toBe(false);
 await act(async () => { dialog.toggle.click(); });
 expect(document.getElementById("create-autonomous-hint")?.textContent).toContain("no checkpoints or questions");
 await dialog.typeTitle("Fast one");
 await dialog.submit();
 expect(dialog.createTicket).toHaveBeenCalledWith(expect.objectContaining({repo_id: 7, title: "Fast one", is_autonomous: true}));
 await dialog.close();
});

it("leaves autopilot off by default", async () => {
 const dialog = await open("repo");
 await dialog.typeTitle("Normal one");
 await dialog.submit();
 expect(dialog.createTicket).toHaveBeenCalledWith(expect.objectContaining({is_autonomous: false}));
 await dialog.close();
});

it("needs a repository before autopilot can be turned on", async () => {
 const dialog = await open("none");
 expect(dialog.toggle.disabled).toBe(true);
 expect(document.getElementById("create-autonomous-hint")?.textContent).toBe("Choose a repository to start it on autopilot.");
 await dialog.close();
});
