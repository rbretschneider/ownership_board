import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import { ApiContext } from "../../api/ApiContext";
import type { BoardApi } from "../../api/boardApi";
import type { TicketDetail } from "../../api/types";
import { CurrentUserContext } from "../../hooks/useCurrentUser";
import { lanes } from "../board/boardFixture";
import { ToastContext } from "../toast/ToastContext";
import { deploymentRepo, deploymentTicket, deploymentUser } from "./deploymentFixture";
import { TicketSheet } from "./TicketSheet";

(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const building: TicketDetail = {
  ...deploymentTicket, lane: "in_progress", stage: "build",
  spec: "Old spec", spec_revision: 1, approved_spec_revision: 1,
  design: "Old design", design_revision: 1, approved_design_revision: 0,
};

function setValue(area: HTMLTextAreaElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")!.set!;
  setter.call(area, value);
  area.dispatchEvent(new Event("input", { bubbles: true }));
}

it("lets the owner edit the spec and design in any stage", async () => {
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  const respec: TicketDetail = { ...building, spec: "New spec", spec_revision: 2 };
  const redesign: TicketDetail = { ...respec, design: "New design", design_revision: 2 };
  const api = {
    ticket: vi.fn().mockResolvedValue(building),
    saveSpec: vi.fn().mockRejectedValueOnce(new Error("Spec too long")).mockResolvedValue(respec),
    saveDesign: vi.fn().mockResolvedValue(redesign),
  };
  const error = vi.fn();
  const success = vi.fn();
  const changed = vi.fn();
  const byLabel = (label: string) => document.querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`);
  const button = (name: string) => [...document.querySelectorAll("button")].find((b) => b.textContent?.trim() === name);
  try {
    await act(async () => root.render(
      <ApiContext.Provider value={api as unknown as BoardApi}>
        <ToastContext.Provider value={{ error, success, info: vi.fn() }}>
          <CurrentUserContext.Provider value={deploymentUser}>
            <TicketSheet ticketId={1} lanes={lanes} stages={[]} repos={[deploymentRepo]} tickets={[building]} refreshSignal={0} onChanged={changed} />
          </CurrentUserContext.Provider>
        </ToastContext.Provider>
      </ApiContext.Provider>,
    ));
    expect(document.body.textContent).toContain("Approved (revision 1)");

    await act(async () => byLabel("Edit spec")!.click());
    const area = document.querySelector<HTMLTextAreaElement>(".section textarea")!;
    expect(area.value).toBe("Old spec");
    expect(document.body.textContent).toContain("Saving makes a new revision");
    await act(async () => setValue(area, "  New spec  "));

    await act(async () => button("Save")!.click());
    expect(api.saveSpec).toHaveBeenLastCalledWith(1, "New spec");
    expect(error).toHaveBeenCalledWith("Spec too long");
    expect(document.querySelector(".section textarea")).not.toBeNull();

    await act(async () => button("Save")!.click());
    expect(success).toHaveBeenCalledWith("Spec updated");
    expect(changed).toHaveBeenCalledTimes(1);
    expect(document.querySelector(".section textarea")).toBeNull();
    expect(document.body.textContent).toContain("New spec");
    expect(document.body.textContent).not.toContain("Approved (revision");

    await act(async () => byLabel("Edit design")!.click());
    await act(async () => setValue(document.querySelector<HTMLTextAreaElement>(".section textarea")!, "New design"));
    await act(async () => button("Save")!.click());
    expect(api.saveDesign).toHaveBeenCalledWith(1, "New design");
    expect(document.body.textContent).toContain("New design");

    await act(async () => byLabel("Edit spec")!.click());
    await act(async () => button("Cancel")!.click());
    expect(api.saveSpec).toHaveBeenCalledTimes(2);
    expect(document.querySelector(".section textarea")).toBeNull();
  } finally {
    await act(async () => root.unmount());
    host.remove();
  }
});
