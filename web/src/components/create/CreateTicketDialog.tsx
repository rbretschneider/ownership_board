import { X } from "lucide-react";
import { useState, type FormEvent, type KeyboardEvent } from "react";
import { useApi } from "../../api/ApiContext";
import type { RepoFilter } from "../../api/boardApi";
import type { Kind, Repo, TicketDetail } from "../../api/types";
import { NO_REPO_LABEL } from "../../lib/constants";
import { decodeRepoFilter, encodeRepoFilter } from "../../lib/repos";
import { preferences, STORAGE_KEYS } from "../../lib/storage";
import { AutosizeTextarea } from "../common/AutosizeTextarea";
import { Overlay } from "../common/Overlay";
import { Spinner } from "../common/Spinner";
import { RepoCombobox } from "../repos/RepoCombobox";
import { KindToggle } from "./KindToggle";

/** Props for {@link CreateTicketDialog}. */
export interface CreateTicketDialogProps {
  repos: Repo[];
  onClose: () => void;
  onCreated: (ticket: TicketDetail) => void;
  /** Preselects the kind (a lane quick-add button); otherwise the last one used. */
  initialKind?: Kind;
}

/** The single create dialog for stories and bugs. */
export function CreateTicketDialog({ repos, onClose, onCreated, initialKind }: CreateTicketDialogProps) {
  const api = useApi();
  const [kind, setKind] = useState<Kind>(
    () => initialKind ?? (preferences.read(STORAGE_KEYS.lastKind, "story") === "bug" ? "bug" : "story"),
  );
  const [repo, setRepo] = useState<RepoFilter>(() => initialRepo(repos));
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [isBusy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const canSubmit = !isBusy && title.trim() !== "";

  const submit = async (event?: FormEvent) => {
    event?.preventDefault();
    if (!canSubmit) {
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const repoId = repo.kind === "repo" ? repo.id : null;
      const ticket = await api.createTicket({ repo_id: repoId, kind, title: title.trim(), body: body.trim() });
      preferences.write(STORAGE_KEYS.lastRepo, encodeRepoFilter(repo));
      preferences.write(STORAGE_KEYS.lastKind, kind);
      onCreated(ticket);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
      setBusy(false);
    }
  };

  const onKeyDown = (event: KeyboardEvent<HTMLFormElement>) => {
    if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
      void submit(event);
    }
  };

  return (
    <Overlay variant="dialog" label="New story or bug" onClose={onClose}>
      <form className="sheet create" onSubmit={(e) => void submit(e)} onKeyDown={onKeyDown} noValidate>
        <header className="sheet__header">
          <h2 className="sheet__title">New {kind === "bug" ? "bug" : "story"}</h2>
          <button type="button" className="icon-button" aria-label="Close" onClick={onClose}>
            <X size={20} aria-hidden="true" />
          </button>
        </header>
        <div className="sheet__body create__body">
          <KindToggle value={kind} onChange={setKind} />
          <div className="field">
            <label className="field__label" htmlFor="create-title">Title</label>
            <input
              id="create-title"
              className="input"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              required
              autoComplete="off"
              enterKeyHint="next"
              data-autofocus
            />
          </div>
          <div className="field">
            <RepoCombobox
              repos={repos}
              value={repo}
              onChange={setRepo}
              label="Repository"
              noneLabel={NO_REPO_LABEL}
              describedBy="create-repo-hint"
            />
            <p id="create-repo-hint" className="field__hint">
              Optional. Without one, the AI works out which repo it belongs to or asks you.
            </p>
          </div>
          <div className="field field--grow">
            <label className="field__label" htmlFor="create-body">Details</label>
            <AutosizeTextarea
              id="create-body"
              value={body}
              onChange={(e) => setBody(e.target.value)}
              placeholder={kind === "bug" ? "What happened, what you expected, how to reproduce…" : "What you want and why. Rough is fine — the AI will ask."}
              aria-describedby="create-body-hint"
            />
            <p id="create-body-hint" className="field__hint">Markdown supported. Ctrl/⌘ + Enter to create.</p>
          </div>
          {error !== null && <p className="form-error" role="alert">Couldn't create it: {error}</p>}
        </div>
        <footer className="sheet__footer">
          <button type="button" className="button button--secondary" onClick={onClose}>Cancel</button>
          <button type="submit" className="button button--primary" disabled={!canSubmit}>
            {isBusy && <Spinner label="Creating" />}
            {isBusy ? "Creating…" : "Create"}
          </button>
        </footer>
      </form>
    </Overlay>
  );
}

/** The last repo used, if it still exists; "no repo" otherwise. */
function initialRepo(repos: Repo[]): RepoFilter {
  const remembered = decodeRepoFilter(preferences.read(STORAGE_KEYS.lastRepo, "none"));
  if (remembered.kind === "repo" && repos.some((r) => r.id === remembered.id && !r.is_archived)) {
    return remembered;
  }
  return { kind: "none" };
}
