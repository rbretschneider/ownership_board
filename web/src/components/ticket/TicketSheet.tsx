import { ArrowLeft, CircleCheck } from "lucide-react";
import { useCallback, useEffect, useRef } from "react";
import { useApi } from "../../api/ApiContext";
import type { Lane, Repo, Stage, TicketDetail, TicketSummary } from "../../api/types";
import { useTicket } from "../../hooks/useTicket";
import { openQuestions } from "../../lib/answers";
import { BOARD_HASH, navigate } from "../../lib/hashRoute";
import { checkpointFor, isApprovedRevision } from "../../lib/stages";
import { ErrorState } from "../common/ErrorState";
import { LoadingState } from "../common/LoadingState";
import { Markdown } from "../common/Markdown";
import { Overlay } from "../common/Overlay";
import { useToast } from "../toast/ToastContext";
import { AnsweredHistory } from "./AnsweredHistory";
import { BlockedPanel } from "./BlockedPanel";
import { CheckpointPanel } from "./CheckpointPanel";
import { CommentComposer } from "./CommentComposer";
import { DecisionsSection } from "./DecisionsSection";
import { EditableMarkdown } from "./EditableMarkdown";
import { PrRow } from "./PrRow";
import { QuestionsPanel } from "./QuestionsPanel";
import { RelatedSection } from "./RelatedSection";
import { TicketHeader } from "./TicketHeader";
import { TicketMenu } from "./TicketMenu";
import { Timeline } from "./Timeline";
import { WorkSection } from "./WorkSection";

/** Props for {@link TicketSheet}. */
export interface TicketSheetProps {
  ticketId: number;
  lanes: Lane[];
  stages: Stage[];
  repos: Repo[];
  /** Every ticket on the board, for the related-ticket picker. */
  tickets: TicketSummary[];
  /** Changes whenever a live event touches this ticket. */
  refreshSignal: number;
  /** Called after any successful change so the board refetches. */
  onChanged: () => void;
}

/** A runner for ticket mutations: applies the result, toasts, reports success. */
export type RunMutation = (action: () => Promise<TicketDetail>, success?: string) => Promise<boolean>;

/** The ticket detail sheet (#/t/:id). */
export function TicketSheet({ ticketId, lanes, stages, repos, tickets, refreshSignal, onChanged }: TicketSheetProps) {
  const api = useApi();
  const toast = useToast();
  const { ticket, error, isMissing, isLoading, reload, accept } = useTicket(ticketId);
  const firstSignal = useRef(refreshSignal);

  useEffect(() => {
    if (refreshSignal !== firstSignal.current) {
      reload();
    }
  }, [refreshSignal, reload]);

  const close = useCallback(() => navigate(BOARD_HASH), []);

  const run = useCallback<RunMutation>(async (action, success) => {
    try {
      accept(await action());
      onChanged();
      if (success !== undefined) {
        toast.success(success);
      }
      return true;
    } catch (cause) {
      toast.error(cause instanceof Error ? cause.message : String(cause));
      return false;
    }
  }, [accept, onChanged, toast]);

  const resolved = useCallback((next: TicketDetail, message: string) => {
    accept(next);
    onChanged();
    toast.success(message);
  }, [accept, onChanged, toast]);

  const open = ticket === null ? [] : openQuestions(ticket.questions);
  const checkpoint = ticket === null ? null : checkpointFor(ticket.stage);

  return (
    <Overlay variant="panel" label={`Ticket #${ticketId}`} onClose={close}>
      <div className="sheet ticket">
        <header className="sheet__header">
          <button type="button" className="icon-button" aria-label="Back to board" onClick={close}>
            <ArrowLeft size={20} aria-hidden="true" />
          </button>
          <h2 className="sheet__title">#{ticketId}</h2>
          <span className="topbar__spacer" />
          {ticket !== null && (
            <TicketMenu
              ticketId={ticketId}
              onDelete={async () => {
                await api.deleteTicket(ticketId);
                toast.success(`Deleted #${ticketId}`);
                onChanged();
                close();
              }}
            />
          )}
        </header>
        <div className="sheet__body ticket__body">
          {ticket === null && isLoading && <LoadingState label="Loading ticket" />}
          {ticket === null && !isLoading && isMissing && (
            <div className="empty-state">
              <p>Ticket #{ticketId} doesn't exist (it may have been deleted).</p>
              <button type="button" className="button button--primary" onClick={close}>Back to board</button>
            </div>
          )}
          {ticket === null && !isLoading && !isMissing && error !== null && (
            <ErrorState message={`Couldn't load #${ticketId}: ${error}`} onRetry={reload} />
          )}
          {ticket !== null && (
            <>
              {checkpoint !== null && (
                <CheckpointPanel
                  key={`${ticket.stage}:${ticket.spec_revision}:${ticket.design_revision}`}
                  ticket={ticket}
                  checkpoint={checkpoint}
                  onResolved={resolved}
                  onReload={reload}
                />
              )}
              {ticket.blocked_reason !== "" && <BlockedPanel key={ticket.blocked_reason} ticket={ticket} run={run} />}
              <TicketHeader ticket={ticket} lanes={lanes} stages={stages} repos={repos} run={run} />
              {open.length > 0 && <QuestionsPanel key={open.map((q) => q.id).join(",")} ticket={ticket} open={open} run={run} />}
              <PrRow ticket={ticket} run={run} />
              <EditableMarkdown
                title="Request"
                value={ticket.body}
                emptyText="No details yet."
                onSave={(body) => run(() => api.editTicket(ticket.id, { body }), "Request updated")}
              />
              <section className="section" aria-labelledby="spec-title">
                <h3 id="spec-title" className="section__title">Spec <span className="section__hint">refined by the AI</span></h3>
                <ApprovedNote approved={ticket.approved_spec_revision} current={ticket.spec_revision} />
                {ticket.spec.trim() === "" ? <p className="muted">Not refined yet.</p> : <Markdown source={ticket.spec} />}
              </section>
              <section className="section" aria-labelledby="design-title">
                <h3 id="design-title" className="section__title">Design <span className="section__hint">written by the AI</span></h3>
                <ApprovedNote approved={ticket.approved_design_revision} current={ticket.design_revision} />
                {ticket.design.trim() === "" ? <p className="muted">No design yet.</p> : <Markdown source={ticket.design} />}
              </section>
              <DecisionsSection ticket={ticket} run={run} />
              <WorkSection ticket={ticket} run={run} />
              <RelatedSection ticket={ticket} tickets={tickets} lanes={lanes} run={run} />
              <AnsweredHistory questions={ticket.questions} />
              <Timeline entries={ticket.timeline} lanes={lanes} stages={stages} />
            </>
          )}
        </div>
        {ticket !== null && (
          <CommentComposer onSend={(body) => run(() => api.comment(ticket.id, body))} />
        )}
      </div>
    </Overlay>
  );
}

/** "Approved (revision N)" when the shown revision is the approved one. */
function ApprovedNote({ approved, current }: { approved: number; current: number }) {
  if (!isApprovedRevision(approved, current)) {
    return null;
  }
  return (
    <p className="approved-note">
      <CircleCheck size={14} aria-hidden="true" />
      Approved (revision {current})
    </p>
  );
}
