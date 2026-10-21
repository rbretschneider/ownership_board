import { useLayoutEffect, useRef, type TextareaHTMLAttributes } from "react";

/** A textarea that grows with its content, up to the CSS max-height. */
export function AutosizeTextarea(props: TextareaHTMLAttributes<HTMLTextAreaElement> & { value: string }) {
  const ref = useRef<HTMLTextAreaElement>(null);
  useLayoutEffect(() => {
    const el = ref.current;
    if (el !== null) {
      el.style.height = "auto";
      el.style.height = `${el.scrollHeight + 2}px`;
    }
  }, [props.value]);
  return <textarea ref={ref} rows={3} {...props} className={`input textarea ${props.className ?? ""}`} />;
}
