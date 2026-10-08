import { useState } from "react";
import { FolderOpen, RotateCcw } from "lucide-react";
import FolderPicker from "../FolderPicker";
import type { ServerSetting } from "../../lib/types";

/**
 * One Settings → Server value. The field holds what's saved here; empty means
 * "use the environment variable, or the default", which the placeholder and
 * the line below say. `draft` is the unsaved edit: a value, null to go back to
 * the environment or default, undefined for none.
 */
export default function ServerSettingField({
  s,
  draft,
  onChange,
}: {
  s: ServerSetting;
  draft: string | null | undefined;
  onChange: (v: string | null) => void;
}) {
  const [browsing, setBrowsing] = useState(false);
  const secret = s.kind === "secret";
  const value = draft === null ? "" : (draft ?? (secret ? "" : (s.value ?? "")));
  const fallback = s.envSet ? (secret ? "set in the environment" : s.env) : s.default;
  const id = `set-${s.key}`;

  let input;
  switch (s.kind) {
    case "choice":
    case "bool": {
      const opts = s.kind === "bool" ? [["true", "On"], ["false", "Off"]] : (s.options ?? []).map((o) => [o, o]);
      const named = (v: string) => opts.find(([o]) => o === v)?.[1] ?? v;
      input = (
        <select id={id} className="field w-full sm:w-64" value={value} onChange={(e) => onChange(e.target.value)}>
          <option value="">Default ({named(fallback)})</option>
          {opts.map(([o, label]) => (
            <option key={o} value={o}>
              {label}
            </option>
          ))}
        </select>
      );
      break;
    }
    case "int":
      input = <input id={id} type="number" min={1} max={64} className="field w-28 tabular-nums" placeholder={fallback} value={value} onChange={(e) => onChange(e.target.value)} />;
      break;
    case "duration":
      input = <input id={id} className="field mono w-28" placeholder={s.placeholder ?? fallback} value={value} onChange={(e) => onChange(e.target.value)} />;
      break;
    case "secret":
      input = (
        <input
          id={id}
          type="password"
          autoComplete="off"
          className="field mono w-full"
          placeholder={s.saved ? "Saved; type to replace" : fallback}
          value={value}
          onChange={(e) => onChange(e.target.value)}
        />
      );
      break;
    default:
      input = (
        <div className="flex gap-2">
          <input
            id={id}
            className={`field min-w-0 flex-1 ${s.kind === "text" ? "" : "mono"}`}
            placeholder={s.placeholder ?? fallback}
            value={value}
            onChange={(e) => onChange(e.target.value)}
          />
          {s.kind === "dir" && (
            <button type="button" className="btn-ghost" onClick={() => setBrowsing((b) => !b)} aria-expanded={browsing}>
              <FolderOpen size={14} /> Browse
            </button>
          )}
        </div>
      );
  }

  return (
    <div>
      <div className="flex items-center gap-2">
        <label className="field-label !mb-0" htmlFor={id}>
          {s.label}
        </label>
        {s.pending && draft === undefined && <span className="badge tint-warning">Restart to apply</span>}
      </div>
      <div className="mt-1.5">{input}</div>
      {browsing && <FolderPicker path={value || (s.envSet ? s.env : "")} onPick={onChange} all />}
      <p className="mt-1 text-xs text-content-muted">{s.help}</p>
      <p className="mt-0.5 flex flex-wrap items-center gap-x-2 text-xs text-content-muted">
        {draft === null ? (
          <span>Goes back to {s.envSet ? <span className="mono">{s.key}</span> : "the default"} when you save.</span>
        ) : s.saved ? (
          <>
            <span>Set here{s.envSet && <>, instead of <span className="mono">{s.key}</span></>}.</span>
            <button type="button" className="inline-flex items-center gap-1 text-accent hover:underline" onClick={() => onChange(null)}>
              <RotateCcw size={11} /> {s.envSet ? "Use the environment's" : "Use the default"}
            </button>
          </>
        ) : s.envSet ? (
          <span>
            From <span className="mono">{s.key}</span>
            {!secret && (
              <>
                {" "}
                = <span className="mono">{s.env}</span>
              </>
            )}
          </span>
        ) : null}
      </p>
    </div>
  );
}
