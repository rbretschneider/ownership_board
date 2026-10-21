/** What copyText needs from the browser, injectable for tests. */
export interface ClipboardEnvironment {
  isSecureContext: boolean;
  clipboard: Pick<Clipboard, "writeText"> | undefined;
  document: Document;
}

/**
 * Copies text to the clipboard and reports whether it worked.
 *
 * The async Clipboard API only exists on secure origins (HTTPS or localhost).
 * A board on a LAN address over plain HTTP has no `navigator.clipboard`, so
 * this falls back to selecting the text in an off-screen textarea and running
 * the legacy copy command, which browsers still allow inside a click handler.
 */
export async function copyText(text: string, env: ClipboardEnvironment = browserEnvironment()): Promise<boolean> {
  if (env.isSecureContext && env.clipboard) {
    try {
      await env.clipboard.writeText(text);
      return true;
    } catch {
      // Permission denied or document not focused: try the legacy path.
    }
  }
  return copyWithSelection(text, env.document);
}

function copyWithSelection(text: string, doc: Document): boolean {
  const area = doc.createElement("textarea");
  area.value = text;
  area.setAttribute("readonly", "");
  area.style.position = "fixed";
  area.style.top = "-1000px";
  area.style.opacity = "0";
  doc.body.appendChild(area);
  const previousFocus = doc.activeElement as HTMLElement | null;
  area.select();
  area.setSelectionRange(0, text.length);
  try {
    return doc.execCommand("copy");
  } catch {
    return false;
  } finally {
    doc.body.removeChild(area);
    previousFocus?.focus();
  }
}

function browserEnvironment(): ClipboardEnvironment {
  return { isSecureContext: window.isSecureContext, clipboard: navigator.clipboard, document };
}
