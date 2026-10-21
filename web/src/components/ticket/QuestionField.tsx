import type { Question } from "../../api/types";
import type { AnswerDraft } from "../../lib/answers";
import { AutosizeTextarea } from "../common/AutosizeTextarea";
import { OptionChip } from "./OptionChip";

/** Radio value for the "your call" choice. */
const DEFER_VALUE = "defer";

/** Props for {@link QuestionField}. */
export interface QuestionFieldProps {
  question: Question;
  number: number;
  draft: AnswerDraft;
  onChange: (draft: AnswerDraft) => void;
}

/** One question: option chips (radios), "your call", and optional free text. */
export function QuestionField({ question, number, draft, onChange }: QuestionFieldProps) {
  const name = `q-${question.id}`;
  const textId = `${name}-text`;
  const selected = draft.deferred ? DEFER_VALUE : draft.choice === null ? "" : String(draft.choice);

  const pick = (value: string) => {
    if (value === DEFER_VALUE) {
      onChange({ ...draft, choice: null, deferred: true });
    } else {
      onChange({ ...draft, choice: Number(value), deferred: false });
    }
  };

  return (
    <fieldset className="question">
      <legend className="question__prompt">
        <span className="question__number">{number}.</span> {question.prompt}
      </legend>
      <div className="question__options">
        {question.options.map((option, index) => (
          <OptionChip
            key={option}
            name={name}
            value={String(index)}
            label={option}
            isSelected={selected === String(index)}
            isRecommended={question.recommended === index}
            onPick={pick}
          />
        ))}
        <OptionChip
          name={name}
          value={DEFER_VALUE}
          label="Your call"
          hint="AI decides"
          isSelected={selected === DEFER_VALUE}
          isRecommended={false}
          onPick={pick}
        />
      </div>
      <label className="field__label field__label--small" htmlFor={textId}>Add detail (optional)</label>
      <AutosizeTextarea
        id={textId}
        rows={1}
        value={draft.text}
        onChange={(e) => onChange({ ...draft, text: e.target.value })}
      />
    </fieldset>
  );
}
