import { useEffect, useState } from "react";
import { Check, Repeat, Trash2 } from "lucide-react";
import { ApiError, api, useApi } from "../../lib/api";
import type { LibraryMatch, Program, RuleMode, RuleSummary, SeriesRule, TvChannel } from "../../lib/types";
import { KEEP_OPTIONS, MODE_TEXT, describeSummary, keepLabel } from "./rules";
import { confirmDialog } from "../../lib/ask";

/** Create or edit the series rule for a program's show. */
export default function SeriesForm({
  program,
  channel,
  onDone,
  onCancel,
}: {
  program: Program;
  channel?: TvChannel;
  onDone: () => void;
  onCancel: () => void;
}) {
  const { data } = useApi<{ candidates: LibraryMatch[]; rule: SeriesRule | null }>(`/api/dvr/rules/options?programId=${program.id}`);
  const rule = data?.rule ?? null;
  const [mode, setMode] = useState<RuleMode>("missing");
  const [onlyHere, setOnlyHere] = useState(false);
  const [itemId, setItemId] = useState<string>("auto");
  const [keep, setKeep] = useState(0);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [result, setResult] = useState<RuleSummary | null>(null);

  // Load an existing rule's settings once.
  useEffect(() => {
    if (!data) return;
    if (rule) {
      setMode(rule.mode);
      setOnlyHere(!!rule.channel);
      setItemId(rule.mediaItemId ? String(rule.mediaItemId) : "none");
      setKeep(rule.keepLast);
    } else if (data.candidates.length) {
      setItemId(String(data.candidates[0].id));
    }
  }, [data, rule]);

  const save = async () => {
    setBusy(true);
    setErr(null);
    const body = {
      programId: program.id,
      mode,
      channel: onlyHere ? program.channel : "",
      mediaItemId: itemId === "none" || itemId === "auto" ? null : Number(itemId),
      keepLast: keep,
      enabled: true,
    };
    try {
      const r = rule
        ? await api<{ summary: RuleSummary }>(`/api/dvr/rules/${rule.id}`, { method: "PUT", json: body })
        : await api<{ summary: RuleSummary }>("/api/dvr/rules", { method: "POST", json: body });
      setResult(r.summary);
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    if (!rule || !(await confirmDialog({ title: `Stop recording "${rule.title}" as a series?`, body: "Upcoming recordings from it are cancelled; finished ones are kept.", action: "Stop recording", danger: true }))) return;
    setBusy(true);
    try {
      await api(`/api/dvr/rules/${rule.id}`, { method: "DELETE" });
      onDone();
    } finally {
      setBusy(false);
    }
  };

  if (result)
    return (
      <div className="mt-4 rounded-md border border-border-light bg-raised p-3">
        <div className="flex items-center gap-2 text-sm font-medium text-content">
          <Check size={15} className="text-good" /> Series recording saved
        </div>
        <p className="mt-1 text-xs text-content-secondary">{describeSummary(result)}</p>
        <p className="mt-1 text-xs text-content-muted">It re-checks whenever the guide refreshes, so later airings are picked up automatically.</p>
        <button className="btn-primary mt-3" onClick={onDone}>
          Done
        </button>
      </div>
    );

  return (
    <div className="mt-4 rounded-md border border-border-light bg-raised p-3">
      <div className="flex items-center gap-2 text-sm font-medium text-content">
        <Repeat size={15} className="text-accent" /> {rule ? "Series recording" : "Record series"}
      </div>
      <div className="mt-3 flex flex-col gap-1.5" role="radiogroup" aria-label="What to record">
        {(Object.keys(MODE_TEXT) as RuleMode[]).map((m) => (
          <label
            key={m}
            className={`flex cursor-pointer gap-2.5 rounded-md border px-3 py-2 transition-colors ${mode === m ? "border-accent bg-surface" : "border-border-light hover:border-border"}`}
          >
            <input type="radio" name="mode" className="accent-brand mt-0.5" checked={mode === m} onChange={() => setMode(m)} />
            <span>
              <span className="block text-sm text-content">{MODE_TEXT[m].label}</span>
              <span className="block text-xs text-content-muted">{MODE_TEXT[m].detail}</span>
            </span>
          </label>
        ))}
      </div>

      <div className="mt-3 grid gap-3 sm:grid-cols-2">
        <div>
          <label className="field-label" htmlFor="rule-ch">
            Channel
          </label>
          <select id="rule-ch" className="field w-full" value={onlyHere ? "here" : "any"} onChange={(e) => setOnlyHere(e.target.value === "here")}>
            <option value="any">Any channel</option>
            <option value="here">Only {channel ? `${channel.number} ${channel.name}` : program.channel}</option>
          </select>
        </div>
        <div>
          <label className="field-label" htmlFor="rule-keep">
            Keep
          </label>
          <select id="rule-keep" className="field w-full" value={keep} onChange={(e) => setKeep(Number(e.target.value))}>
            {KEEP_OPTIONS.map((n) => (
              <option key={n} value={n}>
                {keepLabel(n)}
              </option>
            ))}
          </select>
        </div>
      </div>

      {mode === "missing" && (
        <div className="mt-3">
          <label className="field-label" htmlFor="rule-lib">
            Compare with library show
          </label>
          {data && data.candidates.length === 0 ? (
            <p className="text-xs text-content-muted">Not in your library yet, so it records every episode it hasn't recorded before.</p>
          ) : (
            <select id="rule-lib" className="field w-full" value={itemId} onChange={(e) => setItemId(e.target.value)}>
              {data?.candidates.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.title}
                  {c.year ? ` (${c.year})` : ""} · {c.fileCount} episode{c.fileCount === 1 ? "" : "s"}
                  {c.dvr ? " · DVR recordings" : ""}
                  {c.matching ? ` · ${c.matching} upcoming title${c.matching === 1 ? "" : "s"} match` : ""}
                </option>
              ))}
              <option value="none">Don't compare with the library</option>
            </select>
          )}
        </div>
      )}

      {err && <div className="tint-critical mt-3 rounded-md px-3 py-2 text-xs">{err}</div>}
      <div className="mt-4 flex flex-wrap gap-2">
        <button className="btn-primary" disabled={busy || !data} onClick={() => void save()}>
          {rule ? "Save changes" : "Record series"}
        </button>
        <button className="btn-quiet" onClick={onCancel}>
          Cancel
        </button>
        {rule && (
          <button className="btn-quiet ml-auto hover:!text-critical" disabled={busy} onClick={() => void remove()}>
            <Trash2 size={14} /> Stop series
          </button>
        )}
      </div>
    </div>
  );
}
