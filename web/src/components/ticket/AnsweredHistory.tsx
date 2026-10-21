import type { Question } from "../../api/types";
import { answeredRounds } from "../../lib/answers";

/** Earlier question rounds and their answers, collapsed per round. */
export function AnsweredHistory({ questions }: { questions: Question[] }) {
  const rounds = answeredRounds(questions);
  if (rounds.length === 0) {
    return null;
  }
  return (
    <section className="section" aria-labelledby="qa-history-title">
      <h3 id="qa-history-title" className="section__title">Q&amp;A history</h3>
      {rounds.map(({ round, questions: answered }) => (
        <details key={round} className="qa-round">
          <summary>Round {round} · {answered.length === 1 ? "1 question" : `${answered.length} questions`}</summary>
          <dl className="qa-round__list">
            {answered.map((q) => (
              <div key={q.id} className="qa-round__item">
                <dt>{q.prompt}</dt>
                <dd>{describeAnswer(q)}</dd>
              </div>
            ))}
          </dl>
        </details>
      ))}
    </section>
  );
}

function describeAnswer(question: Question): string {
  const answer = question.answer;
  if (answer === null) {
    return "";
  }
  const parts: string[] = [];
  if (answer.deferred) {
    parts.push("Your call (AI decides)");
  } else if (answer.choice !== null) {
    parts.push(question.options[answer.choice] ?? `Option ${answer.choice + 1}`);
  }
  if (answer.text !== "") {
    parts.push(answer.text);
  }
  return parts.join(" — ");
}
