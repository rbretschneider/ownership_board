import { useCallback, useEffect, useMemo, useState } from "react";
import { ApiContext } from "./api/ApiContext";
import { createBoardApi } from "./api/boardApi";
import { createHttpClient } from "./api/httpClient";
import type { AuthStatus } from "./api/types";
import { LoginForm } from "./components/auth/LoginForm";
import { SetupForm } from "./components/auth/SetupForm";
import { BoardScreen } from "./components/board/BoardScreen";
import { ErrorState } from "./components/common/ErrorState";
import { LoadingState } from "./components/common/LoadingState";
import { useHashRoute } from "./hooks/useHashRoute";
import { BOARD_HASH, navigate } from "./lib/hashRoute";

/** Root component: wires the API, then gates the board behind auth. */
export function App() {
  const [status, setStatus] = useState<AuthStatus | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const route = useHashRoute();

  const api = useMemo(() => {
    const http = createHttpClient({
      fetchImpl: window.fetch.bind(window),
      onSessionExpired: () => setStatus((current) => (current === null ? null : { ...current, user: null })),
    });
    return createBoardApi(http);
  }, []);

  const loadStatus = useCallback(async () => {
    setLoadError(null);
    try {
      setStatus(await api.authStatus());
    } catch (cause) {
      setLoadError(cause instanceof Error ? cause.message : String(cause));
    }
  }, [api]);

  useEffect(() => {
    void loadStatus();
  }, [loadStatus]);

  // Once signed in, the login route has nothing to show; go to the board.
  useEffect(() => {
    if (status?.user != null && route.name === "login") {
      navigate(BOARD_HASH);
    }
  }, [status, route.name]);

  return (
    <ApiContext.Provider value={api}>
      {renderScreen()}
    </ApiContext.Provider>
  );

  function renderScreen() {
    if (status === null) {
      return loadError === null
        ? <main className="splash"><LoadingState label="Loading" /></main>
        : <main className="splash"><ErrorState message={`Couldn't reach the server: ${loadError}`} onRetry={() => void loadStatus()} /></main>;
    }
    if (status.setup_required) {
      return <SetupForm onDone={() => void loadStatus()} />;
    }
    if (status.user === null) {
      const ssoError = route.name === "login" ? route.params.get("error") : null;
      return <LoginForm status={status} ssoError={ssoError} onSignedIn={() => void loadStatus()} />;
    }
    return (
      <BoardScreen
        route={route}
        status={status}
        onSignedOut={() => {
          setStatus({ ...status, user: null });
          navigate(BOARD_HASH);
        }}
        onStatusChanged={() => void loadStatus()}
      />
    );
  }
}
