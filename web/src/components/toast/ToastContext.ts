import { createContext, useContext } from "react";

/** Toast tone. */
export type ToastTone = "success" | "error" | "info";

/** A visible toast. */
export interface Toast {
  id: number;
  tone: ToastTone;
  message: string;
}

/** Shows transient status messages. */
export interface Toaster {
  success(message: string): void;
  error(message: string): void;
  info(message: string): void;
}

/** Provides the {@link Toaster}. */
export const ToastContext = createContext<Toaster | null>(null);

/** The injected {@link Toaster}. Throws outside a provider. */
export function useToast(): Toaster {
  const toaster = useContext(ToastContext);
  if (toaster === null) {
    throw new Error("useToast must be used inside a ToastProvider");
  }
  return toaster;
}
