import { useState, type KeyboardEvent } from "react";

/** The ticket title as a button; clicking turns it into an input that saves on Enter or blur. */
export function EditableTitle({ title, onSave }: { title: string; onSave: (title: string) => Promise<boolean> }) {
  const [draft, setDraft] = useState<string | null>(null);

  const commit = async () => {
    if (draft === null) {
      return;
    }
    const next = draft.trim();
    if (next === "" || next === title) {
      setDraft(null);
      return;
    }
    if (await onSave(next)) {
      setDraft(null);
    }
  };

  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === "Enter") {
      event.preventDefault();
      void commit();
    } else if (event.key === "Escape") {
      event.stopPropagation();
      setDraft(null);
    }
  };

  if (draft === null) {
    return (
      <h1 className="ticket-title">
        <button type="button" className="ticket-title__button" onClick={() => setDraft(title)} title="Edit title">
          {title}
        </button>
      </h1>
    );
  }
  return (
    <div className="field">
      <label className="visually-hidden" htmlFor="ticket-title-input">Title</label>
      <input
        id="ticket-title-input"
        className="input ticket-title__input"
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={() => void commit()}
        onKeyDown={onKeyDown}
        autoFocus
      />
    </div>
  );
}
