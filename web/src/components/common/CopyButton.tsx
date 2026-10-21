import { Check, Copy } from "lucide-react";
import { useState } from "react";
import { copyText } from "../../lib/clipboard";
import { useToast } from "../toast/ToastContext";

const COPIED_RESET_MS = 2000;

/** Copies `text` to the clipboard with visible confirmation. */
export function CopyButton({ text, label }: { text: string; label: string }) {
  const toast = useToast();
  const [isCopied, setCopied] = useState(false);

  const copy = async () => {
    if (await copyText(text)) {
      setCopied(true);
      window.setTimeout(() => setCopied(false), COPIED_RESET_MS);
      return;
    }
    toast.error("Copy failed — select the text and copy it manually.");
  };

  return (
    <button type="button" className="button button--secondary button--small" onClick={() => void copy()} aria-label={label}>
      {isCopied ? <Check size={16} aria-hidden="true" /> : <Copy size={16} aria-hidden="true" />}
      {isCopied ? "Copied" : "Copy"}
    </button>
  );
}
