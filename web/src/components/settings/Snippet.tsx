import { CopyButton } from "../common/CopyButton";

/** A labelled, copyable code block. */
export function Snippet({ label, value }: { label: string; value: string }) {
  return (
    <div className="snippet">
      <div className="snippet__head">
        <span className="field__label field__label--small">{label}</span>
        <CopyButton text={value} label={`Copy ${label.toLowerCase()}`} />
      </div>
      <pre className="snippet__code"><code>{value}</code></pre>
    </div>
  );
}
