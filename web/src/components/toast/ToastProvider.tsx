import { CircleAlert, CircleCheck, Info, X } from "lucide-react";
import { useCallback, useMemo, useRef, useState, type ReactNode } from "react";
import { TOAST_DURATION_MS } from "../../lib/constants";
import { ToastContext, type Toast, type Toaster, type ToastTone } from "./ToastContext";

const ICONS: Record<ToastTone, typeof Info> = { success: CircleCheck, error: CircleAlert, info: Info };

/** Hosts toasts and provides the {@link Toaster} to its children. */
export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const nextId = useRef(1);

  const dismiss = useCallback((id: number) => {
    setToasts((current) => current.filter((t) => t.id !== id));
  }, []);

  const show = useCallback((tone: ToastTone, message: string) => {
    const id = nextId.current++;
    setToasts((current) => [...current, { id, tone, message }]);
    window.setTimeout(() => dismiss(id), tone === "error" ? TOAST_DURATION_MS * 2 : TOAST_DURATION_MS);
  }, [dismiss]);

  const toaster = useMemo<Toaster>(() => ({
    success: (m) => show("success", m),
    error: (m) => show("error", m),
    info: (m) => show("info", m),
  }), [show]);

  return (
    <ToastContext.Provider value={toaster}>
      {children}
      <div className="toasts" role="region" aria-label="Notifications">
        {toasts.map((toast) => {
          const Icon = ICONS[toast.tone];
          return (
            <div key={toast.id} className={`toast toast--${toast.tone}`} role={toast.tone === "error" ? "alert" : "status"}>
              <Icon size={18} aria-hidden="true" />
              <span className="toast__message">{toast.message}</span>
              <button type="button" className="icon-button icon-button--small" aria-label="Dismiss" onClick={() => dismiss(toast.id)}>
                <X size={16} aria-hidden="true" />
              </button>
            </div>
          );
        })}
      </div>
    </ToastContext.Provider>
  );
}
