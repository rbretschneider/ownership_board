import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
import { ApiContext } from "../api/ApiContext";
import type { BoardApi } from "../api/boardApi";
import type { Board } from "../api/types";
import { lanes, prTicket } from "../components/board/boardFixture";
import { useBoard, type BoardState } from "./useBoard";
(globalThis as unknown as {IS_REACT_ACT_ENVIRONMENT: boolean}).IS_REACT_ACT_ENVIRONMENT = true;

it.each([true, false])("moves into Deployed immediately and reconciles success=%s", async success => {
 const original: Board = {lanes, stages: [], repos: [], tickets: [prTicket, {...prTicket, id: 2, lane: "deployed", stage: "deployed"}]};
 const authoritative: Board = {...original, tickets: [{...prTicket, lane: "deployed", stage: "deployed", lease: null}, original.tickets[1]!]};
 let resolve!: () => void;
 let reject!: (error: Error) => void;
 const moveTicket = vi.fn(() => new Promise<void>((yes, no) => {resolve = yes; reject = no;}));
 const board = vi.fn().mockResolvedValueOnce(original).mockResolvedValue(authoritative);
 const api = {board, moveTicket} as unknown as BoardApi;
 let state!: BoardState;
 function Probe() {
  state = useBoard({kind: "all"});
  return <>{state.board?.tickets.map(t => <span key={t.id} data-lane={t.lane}>{t.id}</span>)}</>;
 }
 const host = document.createElement("div");
 const root = createRoot(host);
 await act(async () => root.render(<ApiContext value={api}><Probe /></ApiContext>));
 try {
  let result!: Promise<string | null>;
  await act(async () => {result = state.move(1, "deployed", 2);});
  expect(moveTicket).toHaveBeenCalledWith(1, "deployed", 2);
  expect([...host.querySelectorAll('[data-lane="deployed"]')].map(e => e.textContent)).toEqual(["2", "1"]);
  expect(state.board?.tickets.find(t => t.id === 1)?.stage).toBe("pr");
  expect(board).toHaveBeenCalledTimes(1);
  let error: string | null = null;
  await act(async () => {
   if (success) resolve(); else reject(new Error("move refused"));
   error = await result;
  });
  if (success) {
   expect(error).toBeNull();
   expect(board).toHaveBeenCalledTimes(2);
   expect(state.board).toEqual(authoritative);
  } else {
   expect(error).toBe("move refused");
   expect(state.board).toEqual(original);
   expect(host.querySelector('[data-lane="in_progress"]')?.textContent).toBe("1");
  }
 } finally {await act(async () => root.unmount());}
});
