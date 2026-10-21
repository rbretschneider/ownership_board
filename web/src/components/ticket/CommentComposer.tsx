import { SendHorizontal } from "lucide-react";
import { useState, type KeyboardEvent } from "react";
import { AutosizeTextarea } from "../common/AutosizeTextarea";
import { Spinner } from "../common/Spinner";

/** The comment box pinned to the bottom of the ticket sheet. */
export function CommentComposer({ onSend }: { onSend: (body: string) => Promise<boolean> }) {
  const [body, setBody] = useState("");
  const [isBusy, setBusy] = useState(false);
  const canSend = body.trim() !== "" && !isBusy;

  const send = async () => {
    if (!canSend) {
      return;
    }
    setBusy(true);
    const ok = await onSend(body.trim());
    setBusy(false);
    if (ok) {
      setBody("");
    }
  };

  const onKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
      event.preventDefault();
      void send();
    }
  };

  return (
    <div className="composer">
      <label className="visually-hidden" htmlFor="comment-input">Comment</label>
      <AutosizeTextarea
        id="comment-input"
        rows={1}
        value={body}
        onChange={(e) => setBody(e.target.value)}
        onKeyDown={onKeyDown}
        placeholder="Comment for the AI…"
      />
      <button type="button" className="button button--primary composer__send" onClick={() => void send()} disabled={!canSend} aria-label="Send comment">
        {isBusy ? <Spinner label="Sending" /> : <SendHorizontal size={18} aria-hidden="true" />}
      </button>
    </div>
  );
}
