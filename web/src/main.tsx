import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { ToastProvider } from "./components/toast/ToastProvider";
import { applyStoredTheme } from "./hooks/useTheme";
import "./styles/tokens.css";
import "./styles/base.css";
import "./styles/layout.css";
import "./styles/components.css";
import "./styles/ticket.css";
import "./styles/trace.css";

const SERVICE_WORKER_URL = "/sw.js";

applyStoredTheme();

const root = document.getElementById("root");
if (root === null) {
  throw new Error("index.html is missing #root");
}
createRoot(root).render(
  <StrictMode>
    <ToastProvider>
      <App />
    </ToastProvider>
  </StrictMode>,
);

if (import.meta.env.PROD && "serviceWorker" in navigator) {
  window.addEventListener("load", () => {
    navigator.serviceWorker.register(SERVICE_WORKER_URL).catch((cause: unknown) => {
      // The app works without offline caching; report and carry on.
      console.warn("Service worker registration failed", cause);
    });
  });
}
