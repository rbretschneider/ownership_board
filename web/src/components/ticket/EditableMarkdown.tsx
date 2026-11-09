import { Pencil } from "lucide-react";
import { useId, useState, type ReactNode } from "react";
import { AutosizeTextarea } from "../common/AutosizeTextarea";
import { Markdown } from "../common/Markdown";
import { Spinner } from "../common/Spinner";

/** Props for {@link EditableMarkdown}. */
export interface EditableMarkdownProps {
  title: string;
  /** Small text after the title, such as who usually writes the section. */
  hint?: string;
  value: string;
  emptyText: string;
  /** Shown under the title while reading, such as an approval note. */
  children?: ReactNode;
  /** Shown above the text box while editing, such as what saving changes. */
  editNote?: string;
  /** Persists the new text; resolves true on success. */
  onSave: (value: string) => Promise<boolean>;
}

/** A titled markdown section with Edit → Save/Cancel. */
export function EditableMarkdown({ title, hint, value, emptyText, children, editNote, onSave }: EditableMarkdownProps) {
  const id = useId();
  const [draft, setDraft] = useState<string | null>(null);
  const [isBusy, setBusy] = useState(false);

  const save = async () => {
    if (draft === null) {
      return;
    }
    setBusy(true);
    const ok = await onSave(draft.trim());
    setBusy(false);
    if (ok) {
      setDraft(null);
    }
  };

  return (
    <section className="section" aria-labelledby={`${id}-title`}>
      <div className="section__head">
        <h3 id={`${id}-title`} className="section__title">
          {title}
          {hint !== undefined && <> <span className="section__hint">{hint}</span></>}
        </h3>
        {draft === null && (
          <button type="button" className="button button--ghost button--small" onClick={() => setDraft(value)} aria-label={`Edit ${title.toLowerCase()}`}>
            <Pencil size={14} aria-hidden="true" />
            Edit
          </button>
        )}
      </div>
      {draft === null && children}
      {draft === null && (value.trim() === "" ? <p className="muted">{emptyText}</p> : <Markdown source={value} />)}
      {draft !== null && (
        <div className="field">
          <label className="visually-hidden" htmlFor={`${id}-input`}>{title}</label>
          {editNote !== undefined && <p className="muted">{editNote}</p>}
          <AutosizeTextarea id={`${id}-input`} value={draft} onChange={(e) => setDraft(e.target.value)} autoFocus />
          <div className="row-actions">
            <button type="button" className="button button--secondary" onClick={() => setDraft(null)}>Cancel</button>
            <button type="button" className="button button--primary" onClick={() => void save()} disabled={isBusy}>
              {isBusy && <Spinner label="Saving" />}
              Save
            </button>
          </div>
        </div>
      )}
    </section>
  );
}
