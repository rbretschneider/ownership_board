import { Bot } from "lucide-react";
import { EditableTitle } from "./EditableTitle";
import { useApi } from "../../api/ApiContext";
import type { RepoFilter } from "../../api/boardApi";
import type { Lane, LaneId, Repo, Stage, TicketDetail } from "../../api/types";
import { NO_REPO_LABEL, OWNER_MOVE_LABELS } from "../../lib/constants";
import { relativeTime } from "../../lib/relativeTime";
import { KindTag } from "../common/KindTag";
import { OwnerBadge } from "../common/OwnerBadge";
import { StageChip } from "../common/StageChip";
import { RepoCombobox } from "../repos/RepoCombobox";
import type { RunMutation } from "./TicketSheet";

/** Stages where a person may turn a ticket autonomous (domain Stage.MayTurnAutonomous). */
const AUTONOMOUS_STAGES: readonly string[] = ["ready", "design", "build"];

/** Props for {@link TicketHeader}. */
export interface TicketHeaderProps {
  ticket: TicketDetail;
  lanes: Lane[];
  stages: Stage[];
  repos: Repo[];
  run: RunMutation;
}

/** Title (click to edit), owner, lane, stage and repository controls. */
export function TicketHeader({ ticket, lanes, stages, repos, run }: TicketHeaderProps) {
  const api = useApi();
  const repoValue: RepoFilter = ticket.repo_id === null ? { kind: "none" } : { kind: "repo", id: ticket.repo_id };
  const laneLabel = (id: LaneId) => lanes.find((l) => l.id === id)?.label ?? id;

  const moveTo = (lane: LaneId) => {
    void run(() => api.moveTicket(ticket.id, lane, null), `Moved to ${laneLabel(lane)}`);
  };
  const changeRepo = (value: RepoFilter) => {
    const repoId = value.kind === "repo" ? value.id : null;
    if (repoId !== ticket.repo_id) {
      void run(() => api.editTicket(ticket.id, { repo_id: repoId }), "Repository updated");
    }
  };
  // The board refuses to turn autonomy on before the spec is past review or
  // while the ticket waits at a checkpoint.
  const mayTurnAutonomous = AUTONOMOUS_STAGES.includes(ticket.stage);
  const toggleAutonomous = (next: boolean) => {
    void run(
      () => api.editTicket(ticket.id, { is_autonomous: next }),
      next ? "Autonomous: runs to a pull request" : "Autonomy off: checkpoints apply again",
    );
  };

  return (
    <div className="ticket-header">
      <div className="ticket-header__tags">
        <KindTag kind={ticket.kind} />
        <OwnerBadge owner={ticket.owner} label={OWNER_MOVE_LABELS[ticket.owner]} />
        <span className="muted small">Created {relativeTime(ticket.created_at)} by {ticket.created_by}</span>
      </div>
      <EditableTitle
        title={ticket.title}
        onSave={(title) => run(() => api.editTicket(ticket.id, { title }), "Title updated")}
      />
      {ticket.lease !== null && (
        <p className="lease" role="status">
          <span className="pulse" aria-hidden="true" />
          <Bot size={16} aria-hidden="true" />
          {ticket.lease.holder} is working on this
        </p>
      )}
      <div className="ticket-header__controls">
        <div className="field">
          <label className="field__label" htmlFor="ticket-lane">Move to</label>
          <div className="lane-move">
            <select id="ticket-lane" className="input select" value={ticket.lane} onChange={(e) => moveTo(e.target.value as LaneId)}>
              {lanes.map((lane) => (
                <option key={lane.id} value={lane.id}>{lane.label}</option>
              ))}
            </select>
            <span className="lane-move__stage">
              <span className="visually-hidden">Stage: </span>
              <StageChip stage={ticket.stage} stages={stages} />
            </span>
          </div>
        </div>
        <div className="field">
          <RepoCombobox repos={repos} value={repoValue} onChange={changeRepo} label="Repository" noneLabel={NO_REPO_LABEL} />
        </div>
      </div>
      <label className="switch" htmlFor="ticket-autonomous">
        <input
          id="ticket-autonomous"
          type="checkbox"
          role="switch"
          className="switch__input"
          checked={ticket.is_autonomous}
          disabled={!ticket.is_autonomous && !mayTurnAutonomous}
          onChange={(e) => toggleAutonomous(e.target.checked)}
          aria-describedby="ticket-autonomous-hint"
        />
        <span className="switch__track" aria-hidden="true" />
        <span className="switch__label">Autonomous</span>
      </label>
      <p id="ticket-autonomous-hint" className="muted small">
        {ticket.is_autonomous
          ? "No checkpoints or questions: agents decide and you review the pull request."
          : mayTurnAutonomous
            ? "Agents stop at this repo's checkpoints and ask when unsure."
            : "Agents stop at this repo's checkpoints and ask when unsure. Autonomy can be turned on once the spec is approved."}
      </p>
    </div>
  );
}
