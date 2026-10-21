import { useMemo } from "react";
import { renderMarkdown } from "../../lib/markdown";

/** Renders sanitized markdown. */
export function Markdown({ source }: { source: string }) {
  const html = useMemo(() => renderMarkdown(source), [source]);
  return <div className="markdown" dangerouslySetInnerHTML={{ __html: html }} />;
}
