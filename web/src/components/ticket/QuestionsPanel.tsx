import { MessageCircleQuestion, Sparkles } from "lucide-react";
import { useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { Question, TicketDetail } from "../../api/types";
import {
  acceptRecommendations,
  areAllAnswered,
  EMPTY_DRAFT,
  ownDraft,
  roundSummary,
  toAnswers,
  type AnswerDraft,
  type DraftMap,
} from "../../lib/answers";
import { Spinner } from "../common/Spinner";
import { QuestionField } from "./QuestionField";
import type { RunMutation } from "./TicketSheet";

/** Props for {@link QuestionsPanel}. */
export interface QuestionsPanelProps {
  ticket: TicketDetail;
  open: Question[];
  run: RunMutation;
}

/** The open question round: answer each by tapping an option, typing, or "your call". */
export function QuestionsPanel({ ticket, open, run }: QuestionsPanelProps) {
  const api = useApi();
  const [drafts, setDrafts] = useState<DraftMap>({});
  const [isBusy, setBusy] = useState(false);
  const round = Math.max(...open.map((q) => q.round));
  const summary = roundSummary(ticket.timeline, round);
  const isComplete = areAllAnswered(open, drafts);
  const remaining = open.length - open.filter((q) => areAllAnswered([q], drafts)).length;

  const update = (questionId: number, draft: AnswerDraft) => {
    setDrafts((current) => ({ ...current, [questionId]: ownDraft(draft) }));
  };

  const send = async (next: DraftMap) => {
    setBusy(true);
    const ok = await run(() => api.answer(ticket.id, toAnswers(open, next)), "Answers sent — back to the AI");
    if (!ok) {
      setBusy(false);
    }
  };

  return (
    <section className="questions" aria-labelledby="questions-title">
      <header className="questions__header">
        <MessageCircleQuestion size={20} aria-hidden="true" />
        <h3 id="questions-title" className="questions__title">
          The AI has {open.length === 1 ? "a question" : `${open.length} questions`}
        </h3>
      </header>
      {summary !== "" && <p className="questions__summary">{summary}</p>}
      <ol className="questions__list">
        {open.map((question, index) => (
          <li key={question.id}>
            <QuestionField
              question={question}
              number={index + 1}
              draft={drafts[question.id] ?? EMPTY_DRAFT}
              onChange={(draft) => update(question.id, draft)}
            />
          </li>
        ))}
      </ol>
      <div className="questions__actions">
        <button
          type="button"
          className="button button--secondary"
          disabled={isBusy}
          onClick={() => setDrafts(acceptRecommendations(open, drafts))}
        >
          <Sparkles size={16} aria-hidden="true" />
          Accept recommendations
        </button>
        <button type="button" className="button button--primary" disabled={!isComplete || isBusy} onClick={() => void send(drafts)}>
          {isBusy && <Spinner label="Sending" />}
          {isBusy ? "Sending…" : "Send answers"}
        </button>
      </div>
      {!isComplete && !isBusy && (
        <p className="field__hint" aria-live="polite">
          {remaining === 1 ? "1 question still needs an answer." : `${remaining} questions still need an answer.`}
        </p>
      )}
    </section>
  );
}
