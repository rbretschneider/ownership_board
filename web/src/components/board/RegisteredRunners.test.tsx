import { expect, it } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { RegisteredRunnersContext } from "../../hooks/useRunners";
import type { RegisteredRunnersSnapshot } from "../../api/types";
import { RunnersStrip } from "./RunnersStrip";
const hosts: RegisteredRunnersSnapshot = {
  observed_at: "2026-01-01T00:00:00Z",
  queued_by_os: { windows: 2, linux: 0, macos: 1 },
  hosts: [
    {
      id: 1,
      name: "Win builder",
      os: "windows",
      enabled: true,
      max_parallel: 1,
      online: false,
      last_seen_at: null,
      jobs: [],
    },
    {
      id: 2,
      name: "Linux builder",
      os: "linux",
      enabled: false,
      max_parallel: 1,
      online: true,
      last_seen_at: "2026-01-01",
      jobs: [
        {
          id: "a",
          ticket_id: 12,
          repo: "acme/app",
          sha: "abc",
          preset: "cmake",
          status: "cancelled",
          phase: "build",
          detail: "",
          started_at: null,
          attempt: 1,
        },
        {
          id: "b",
          ticket_id: 13,
          repo: "acme/app",
          sha: "def",
          preset: "cmake",
          status: "running",
          phase: "test",
          detail: "tests",
          started_at: null,
          attempt: 1,
        },
      ],
    },
    {
      id: 3,
      name: "Mac builder",
      os: "macos",
      enabled: true,
      max_parallel: 2,
      online: true,
      last_seen_at: "2026-01-01",
      jobs: [],
    },
  ],
};
function dom(stale = false) {
  const d = document.createElement("div");
  d.innerHTML = renderToStaticMarkup(
    <RegisteredRunnersContext value={{ snapshot: hosts, stale, error: null, refresh: () => {} }}>
      <RunnersStrip />
    </RegisteredRunnersContext>,
  );
  return d;
}
it("shows registered host connectivity, OS queues, all occupied slots and ticket links", () => {
  const d = dom();
  expect(d.querySelectorAll(".registered-runner")).toHaveLength(3);
  expect(d.textContent).toContain("Windows queue: 2");
  expect(d.textContent).toContain("never seen");
  expect(d.querySelector(".registered-runner--warning")?.textContent).toContain("Offline with queued work");
  expect(d.textContent).toContain("Disabled");
  expect(d.textContent).toContain("stopping");
  expect(d.querySelector('a[href="#/t/12"]')).not.toBeNull();
  expect(d.querySelector('a[href="#/t/13"]')).not.toBeNull();
  expect(d.querySelectorAll(".registered-runner")[1]!.querySelectorAll("a")).toHaveLength(2);
});
it("marks retained readings stale separately from reported connectivity", () => {
  const d = dom(true);
  expect(d.textContent).toContain("Status reading is stale");
  expect(d.textContent).toContain("Last reported online");
  expect(d.textContent).not.toContain("Offline with queued work");
});
