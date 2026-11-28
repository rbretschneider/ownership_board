import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import type { BoardApi } from "../../api/boardApi";
import { ProviderTokenForm } from "./ProviderTokenForm";
import { FORGEJO_FORM } from "./providerForms";
(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

function type(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!;
  setter.call(input, value);
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

async function mount(onSubmit: ReturnType<typeof vi.fn>) {
  const host = document.createElement("div");
  document.body.append(host);
  const root = createRoot(host);
  await act(async () =>
    root.render(
      <ProviderTokenForm idPrefix="t" title="Connect Forgejo" field={FORGEJO_FORM.field} tokenHint={FORGEJO_FORM.tokenHint}
        option={FORGEJO_FORM.option} submitLabel="Connect" onSubmit={onSubmit} onConnected={() => undefined} />,
    ),
  );
  const fill = async () => {
    await act(async () => {
      type(host.querySelector<HTMLInputElement>("#t-account")!, "http://10.0.10.11:3021");
      type(host.querySelector<HTMLInputElement>("#t-token")!, "fj_token");
    });
  };
  const submit = () => act(async () => host.querySelector("form")!.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true })));
  return { host, fill, submit, unmount: async () => { await act(async () => root.unmount()); host.remove(); } };
}

it("keeps agent access off unless the person turns it on", async () => {
  const onSubmit = vi.fn().mockResolvedValue({});
  const view = await mount(onSubmit);
  try {
    const option = view.host.querySelector<HTMLInputElement>("#t-option")!;
    expect(option.getAttribute("role")).toBe("switch");
    expect(option.checked).toBe(false);
    await view.fill();
    await view.submit();
    expect(onSubmit).toHaveBeenLastCalledWith("http://10.0.10.11:3021", "fj_token", false);
    await act(async () => option.click());
    await view.fill();
    await view.submit();
    expect(onSubmit).toHaveBeenLastCalledWith("http://10.0.10.11:3021", "fj_token", true);
  } finally {
    await view.unmount();
  }
});

it("sends the opt-in to the board with the Forgejo connection", async () => {
  const connectForgejo = vi.fn().mockResolvedValue({});
  await FORGEJO_FORM.submit({ connectForgejo } as unknown as BoardApi, "https://git.example.com", "fj", true);
  expect(connectForgejo).toHaveBeenCalledWith({ base_url: "https://git.example.com", token: "fj", allow_agent_access: true });
});
