import { Plus } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import type { RepoFilter } from "../../api/boardApi";
import type { AuthStatus, BoardEvent, Kind, Lane, LaneId, TicketSummary } from "../../api/types";
import { useBoard } from "../../hooks/useBoard";
import { useDebouncedCallback } from "../../hooks/useDebouncedCallback";
import { useDragDrop } from "../../hooks/useDragDrop";
import { useEventStream } from "../../hooks/useEventStream";
import { useMediaQuery } from "../../hooks/useMediaQuery";
import { DEFAULT_LANE, DESKTOP_MEDIA_QUERY, REFETCH_DEBOUNCE_MS } from "../../lib/constants";
import { laneCardIds, type DropTarget } from "../../lib/dropTargets";
import type { Route } from "../../lib/hashRoute";
import { afterIdFor, groupByLane } from "../../lib/position";
import { decodeRepoFilter, encodeRepoFilter } from "../../lib/repos";
import { needsOwner } from "../../lib/stages";
import { preferences, STORAGE_KEYS } from "../../lib/storage";
import { ErrorState } from "../common/ErrorState";
import { LoadingState } from "../common/LoadingState";
import { CreateTicketDialog } from "../create/CreateTicketDialog";
import { SettingsSheet } from "../settings/SettingsSheet";
import { TicketSheet } from "../ticket/TicketSheet";
import { useToast } from "../toast/ToastContext";
import { DesktopBoard } from "./DesktopBoard";
import { MobileBoard } from "./MobileBoard";
import { TicketCardContent } from "./TicketCardContent";
import { TopBar } from "./TopBar";

/** Props for {@link BoardScreen}. */
export interface BoardScreenProps {
  route: Route;
  status: AuthStatus;
  onSignedOut: () => void;
  onStatusChanged: () => void;
}

/** The signed-in app: board, create dialog, ticket sheet and settings. */
export function BoardScreen({ route, status, onSignedOut, onStatusChanged }: BoardScreenProps) {
  const toast = useToast();
  const isDesktop = useMediaQuery(DESKTOP_MEDIA_QUERY);
  const [repoFilter, setRepoFilter] = useState<RepoFilter>(() => decodeRepoFilter(preferences.read(STORAGE_KEYS.repoFilter, "")));
  const [isYourMoveOnly, setYourMoveOnly] = useState(false);
  // The open create dialog; kind is set when a quick-add button picked it.
  const [createRequest, setCreateRequest] = useState<{ kind?: Kind } | null>(null);
  const [chosenLane, setChosenLane] = useState<LaneId | null>(null);
  const [ticketSignal, setTicketSignal] = useState(0);
  const { board, error, isLoading, refetch, move } = useBoard(repoFilter);

  const debouncedRefetch = useDebouncedCallback(refetch, REFETCH_DEBOUNCE_MS);
  const onEvent = useCallback((event: BoardEvent) => {
    debouncedRefetch();
    if (route.name === "ticket" && route.ticketId === event.ticket_id) {
      setTicketSignal((n) => n + 1);
    }
  }, [debouncedRefetch, route]);
  const onResync = useCallback(() => {
    refetch();
    setTicketSignal((n) => n + 1);
  }, [refetch]);
  const isConnected = useEventStream(onEvent, onResync);

  const onDrop = useCallback((ticketId: number, target: DropTarget) => {
    const afterId = target.kind === "pill" ? null : afterIdFor(laneCardIds(target.lane), ticketId, target.index);
    if (board === null || isNoOpMove(board.tickets, ticketId, target.lane, afterId)) {
      return;
    }
    void move(ticketId, target.lane, afterId).then((failure) => {
      if (failure !== null) {
        toast.error(`Couldn't move #${ticketId}: ${failure}`);
      }
    });
  }, [board, move, toast]);
  const drag = useDragDrop(onDrop);

  const changeRepoFilter = useCallback((filter: RepoFilter) => {
    preferences.write(STORAGE_KEYS.repoFilter, encodeRepoFilter(filter));
    setRepoFilter(filter);
  }, []);

  const lanes = board?.lanes ?? [];
  const repos = board?.repos ?? [];
  const allTickets = board?.tickets ?? [];
  const stages = board?.stages ?? [];
  const yourMoveCount = needsOwner(allTickets).length;
  const visible = useMemo(
    () => (isYourMoveOnly ? needsOwner(allTickets) : allTickets),
    [allTickets, isYourMoveOnly],
  );
  const ticketsByLane = useMemo(() => groupByLane(visible, lanes.map((l) => l.id)), [visible, lanes]);
  const reposById = useMemo(() => new Map(repos.map((r) => [r.id, r])), [repos]);
  const draggedTicket = drag.session === null ? undefined : allTickets.find((t) => t.id === drag.session?.ticketId);

  // Pick the starting lane once, on first load, so refetches never yank the view.
  useEffect(() => {
    if (board !== null && chosenLane === null) {
      setChosenLane(defaultLane(board.lanes, board.tickets));
    }
  }, [board, chosenLane]);

  // A remembered filter for a repo that no longer exists falls back to all repos.
  useEffect(() => {
    if (board !== null && repoFilter.kind === "repo" && !board.repos.some((r) => r.id === repoFilter.id)) {
      changeRepoFilter({ kind: "all" });
    }
  }, [board, repoFilter, changeRepoFilter]);

  const viewProps = {
    lanes,
    stages,
    ticketsByLane,
    reposById,
    draggedId: drag.session?.ticketId ?? null,
    target: drag.session?.target ?? null,
    onPointerDown: drag.startPress,
    shouldIgnoreClick: drag.consumeClick,
    onQuickAdd: (kind: Kind) => setCreateRequest({ kind }),
  };

  return (
    <div className={`app${isDesktop ? " app--desktop" : " app--mobile"}`}>
      <TopBar
        repos={repos}
        repoFilter={repoFilter}
        onRepoFilterChange={changeRepoFilter}
        yourMoveCount={yourMoveCount}
        isYourMoveOnly={isYourMoveOnly}
        onToggleYourMove={() => setYourMoveOnly((v) => !v)}
        isConnected={isConnected}
        showNewButton={isDesktop}
        onNew={() => setCreateRequest({})}
      />
      <main className="board" aria-busy={isLoading}>
        {board === null && isLoading && <LoadingState label="Loading board" />}
        {board === null && !isLoading && error !== null && (
          <ErrorState message={`Couldn't load the board: ${error}`} onRetry={refetch} />
        )}
        {board !== null && error !== null && (
          <p className="banner banner--error" role="alert">
            Couldn't refresh the board: {error}{" "}
            <button type="button" className="link-button" onClick={refetch}>Retry</button>
          </p>
        )}
        {board !== null && isDesktop && <DesktopBoard {...viewProps} />}
        {board !== null && !isDesktop && (
          <MobileBoard {...viewProps} activeLane={chosenLane ?? DEFAULT_LANE} onSelectLane={setChosenLane} />
        )}
      </main>
      {!isDesktop && (
        <button type="button" className="fab" aria-label="New story or bug" onClick={() => setCreateRequest({})}>
          <Plus size={26} aria-hidden="true" />
        </button>
      )}
      {drag.session !== null && draggedTicket !== undefined && (
        <div ref={drag.ghostRef} className="drag-ghost" style={{ width: drag.session.width }} aria-hidden="true">
          <div className="card card--ghost">
            <TicketCardContent ticket={draggedTicket} stages={stages} repo={draggedTicket.repo_id === null ? undefined : reposById.get(draggedTicket.repo_id)} />
          </div>
        </div>
      )}
      {createRequest !== null && (
        <CreateTicketDialog
          repos={repos}
          initialKind={createRequest.kind}
          onClose={() => setCreateRequest(null)}
          onCreated={(ticket) => {
            setCreateRequest(null);
            toast.success(`Created #${ticket.id}`);
            refetch();
          }}
        />
      )}
      {route.name === "ticket" && route.ticketId !== null && (
        <TicketSheet
          key={route.ticketId}
          ticketId={route.ticketId}
          lanes={lanes}
          stages={stages}
          repos={repos}
          tickets={board?.tickets ?? []}
          refreshSignal={ticketSignal}
          onChanged={refetch}
        />
      )}
      {route.name === "settings" && (
        <SettingsSheet
          route={route}
          status={status}
          onSignedOut={onSignedOut}
          onChanged={() => {
            refetch();
            onStatusChanged();
          }}
        />
      )}
    </div>
  );
}

function isNoOpMove(tickets: TicketSummary[], ticketId: number, lane: LaneId, afterId: number | null): boolean {
  const ticket = tickets.find((t) => t.id === ticketId);
  if (ticket === undefined || ticket.lane !== lane) {
    return false;
  }
  const laneIds = laneCardIds(lane);
  const index = laneIds.indexOf(ticketId);
  const currentAfter = index > 0 ? laneIds[index - 1] ?? null : null;
  return currentAfter === afterId;
}

/** The first lane holding a ticket that needs the owner, else Todo. */
function defaultLane(lanes: Lane[], tickets: TicketSummary[]): LaneId {
  const waiting = needsOwner(tickets);
  return lanes.find((l) => waiting.some((t) => t.lane === l.id))?.id ?? DEFAULT_LANE;
}
