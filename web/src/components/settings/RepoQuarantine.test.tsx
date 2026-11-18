import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import { ApiContext } from "../../api/ApiContext";
import type { BoardApi } from "../../api/boardApi";
import type { QuarantinedTest } from "../../api/types";
import { repo as makeRepo } from "../../lib/flowFixture";
import { ToastProvider } from "../toast/ToastProvider";
import { RepoQuarantine } from "./RepoQuarantine";
(globalThis as unknown as {IS_REACT_ACT_ENVIRONMENT: boolean}).IS_REACT_ACT_ENVIRONMENT = true;

const entry = (over: Partial<QuarantinedTest>): QuarantinedTest => ({
 repo_id: 1, test: "gl:Native Fillet hovers", status: "active", source: "seeded", added_by: "Clod", reason: "failed on 4 tickets",
 evidence: {tickets: [1, 2]}, fix_ticket_id: 230, passes: 0, created_at: "", updated_at: "", released_at: null, ...over,
});

it("loads the list when opened, shows who added each test and its fix, and releases one", async () => {
 const quarantine = vi.fn().mockResolvedValue({tests: [entry({}), entry({test: "ui:wheel zoom", status: "probation", source: "detected", passes: 2, fix_ticket_id: null})]});
 const releaseQuarantine = vi.fn().mockResolvedValue(entry({status: "released"}));
 const host = document.createElement("div"); document.body.append(host);
 const root = createRoot(host);
 await act(async () => root.render(<ApiContext value={{quarantine, releaseQuarantine} as unknown as BoardApi}><ToastProvider><RepoQuarantine repo={makeRepo(1, "app")} /></ToastProvider></ApiContext>));
 expect(quarantine).not.toHaveBeenCalled();
 const details = host.querySelector("details")!;
 await act(async () => { details.open = true; details.dispatchEvent(new Event("toggle")); });
 expect(quarantine).toHaveBeenCalledWith(1);
 expect(host.querySelector("summary")?.textContent).toBe("Quarantined tests (2)");
 const items = [...host.querySelectorAll("li")];
 expect(items[0]?.textContent).toContain("Seeded by Clod");
 expect(items[0]?.querySelector("a")?.getAttribute("href")).toBe("#/t/230");
 expect(items[1]?.textContent).toContain("Fix merged, running again");
 expect(items[1]?.textContent).toContain("2 passing runs");
 await act(async () => items[0]!.querySelector("button")!.click());
 expect(releaseQuarantine).toHaveBeenCalledWith(1, "gl:Native Fillet hovers");
 expect(host.querySelector("summary")?.textContent).toBe("Quarantined tests (1)");
 await act(async () => root.unmount()); host.remove();
});
