import { Check } from "lucide-react";
import { useState } from "react";
import { useApi } from "../../api/ApiContext";
import type { Repo } from "../../api/types";
import { REPO_COLORS } from "../../lib/constants";
import { Spinner } from "../common/Spinner";
import { useToast } from "../toast/ToastContext";

const AUTO_COLOR = "";

/** Named color swatches (plus "Auto") for one repo. */
export function RepoColorPicker({ repo, onSaved }: { repo: Repo; onSaved: (repo: Repo) => void }) {
  const api = useApi();
  const toast = useToast();
  const [busyValue, setBusyValue] = useState<string | null>(null);
  const choices = [{ name: "Auto", value: AUTO_COLOR }, ...REPO_COLORS];

  const choose = async (color: string) => {
    setBusyValue(color);
    try {
      onSaved(await api.editRepo(repo.id, { color }));
    } catch (cause) {
      toast.error(`Couldn't change the color: ${cause instanceof Error ? cause.message : String(cause)}`);
    } finally {
      setBusyValue(null);
    }
  };

  return (
    <fieldset className="swatches">
      <legend className="field__label field__label--small">Color for {repo.full_name}</legend>
      {choices.map((choice) => {
        const isCurrent = repo.color === choice.value;
        return (
          <button
            key={choice.name}
            type="button"
            className={`swatch${isCurrent ? " swatch--current" : ""}${choice.value === AUTO_COLOR ? " swatch--auto" : ""}`}
            style={choice.value === AUTO_COLOR ? undefined : { background: choice.value }}
            aria-pressed={isCurrent}
            aria-label={choice.name}
            title={choice.name}
            disabled={busyValue !== null}
            onClick={() => void choose(choice.value)}
          >
            {busyValue === choice.value ? <Spinner label="Saving" size={14} /> : isCurrent ? <Check size={14} aria-hidden="true" /> : null}
            {choice.value === AUTO_COLOR && busyValue !== choice.value && !isCurrent ? <span aria-hidden="true">A</span> : null}
          </button>
        );
      })}
    </fieldset>
  );
}
