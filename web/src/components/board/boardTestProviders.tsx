import type { ReactNode } from "react";
import { vi } from "vitest";
import { ApiContext } from "../../api/ApiContext";
import { createBoardApi } from "../../api/boardApi";
import { ToastContext } from "../toast/ToastContext";

const api = createBoardApi({request: vi.fn()});
const toast = {error: vi.fn(), success: vi.fn(), info: vi.fn()};
export function BoardTestProviders({children}: {children: ReactNode}) {
  return <ApiContext.Provider value={api}><ToastContext.Provider value={toast}>{children}</ToastContext.Provider></ApiContext.Provider>;
}
