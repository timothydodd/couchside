import { useEffect, useState } from "react";
import { api, useApi } from "../../lib/api";

interface Timing {
  padBefore: number;
  padAfter: number;
  skipAfterStart: number;
  skipBeforeEnd: number;
  mergeGap: number;
  dvr: boolean;
}

type Field = Exclude<keyof Timing, "dvr">;

/**
 * Settings → Advanced: recording padding and how much of each commercial
 * break skipping leaves in. Shared by every profile.
 */
export default function TimingSettings() {
  return (
    <section className="card p-4">
      <div className="card-title mb-3">Timing</div>
      <TimingForm />
    </section>
  );
}

function TimingForm() {
  const { data, reload } = useApi<Timing>("/api/settings/timing");
  const [draft, setDraft] = useState<Timing | null>(null);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ tone: "good" | "critical"; text: string } | null>(null);
  useEffect(() => setDraft(data ?? null), [data]);
  if (!data || !draft) return null;

  const changed = (Object.keys(data) as (keyof Timing)[]).some((k) => data[k] !== draft[k]);
  const set = (k: Field, v: string) => {
    setMsg(null);
    setDraft({ ...draft, [k]: v === "" ? 0 : Number(v) });
  };
  const save = async () => {
    setBusy(true);
    setMsg(null);
    try {
      await api("/api/settings/timing", { method: "PUT", json: draft });
      setMsg({ tone: "good", text: draft.dvr ? "Saved. Upcoming recordings use the new padding." : "Saved." });
      await reload();
    } catch (e) {
      setMsg({ tone: "critical", text: e instanceof Error ? e.message : String(e) });
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <div className="grid gap-x-6 gap-y-4 sm:grid-cols-2">
        {draft.dvr && (
          <div>
            <div className="field-label">Recording padding</div>
            <Seconds label="Start early" value={draft.padBefore} max={1800} step={5} onChange={(v) => set("padBefore", v)} />
            <Seconds label="Keep going after" value={draft.padAfter} max={1800} step={5} onChange={(v) => set("padAfter", v)} />
            <p className="mt-1 text-xs text-content-muted">Catches shows that start early or run over. Applies to new and upcoming recordings.</p>
          </div>
        )}
        <div>
          <div className="field-label">Commercial skipping</div>
          <Seconds label="Start skipping" suffix="into a break" value={draft.skipAfterStart} max={15} step={0.5} onChange={(v) => set("skipAfterStart", v)} />
          <Seconds label="Stop skipping" suffix="before it ends" value={draft.skipBeforeEnd} max={15} step={0.5} onChange={(v) => set("skipBeforeEnd", v)} />
          <Seconds label="Join breaks" suffix="apart or less" value={draft.mergeGap} max={300} step={5} onChange={(v) => set("mergeGap", v)} />
          <p className="mt-1 text-xs text-content-muted">
            Detection is often a second off; the first two keep skips from cutting into the show. Detection can also split one break in two around a
            promo; breaks this close are skipped as one.
          </p>
        </div>
      </div>
      <div className="mt-4 flex items-center gap-3">
        <button className="btn-primary" disabled={!changed || busy} onClick={() => void save()}>
          Save
        </button>
        {changed && (
          <button className="btn-quiet" onClick={() => setDraft(data)}>
            Cancel
          </button>
        )}
        {msg && <span className={`text-xs ${msg.tone === "good" ? "text-good" : "text-critical"}`}>{msg.text}</span>}
      </div>
    </>
  );
}

function Seconds({ label, suffix, value, max, step, onChange }: { label: string; suffix?: string; value: number; max: number; step: number; onChange: (v: string) => void }) {
  return (
    <label className="mt-1.5 flex items-center gap-2 text-sm">
      <span className="w-32 shrink-0 text-content-secondary">{label}</span>
      <input type="number" className="field w-20 !py-1 text-right tabular-nums" min={0} max={max} step={step} value={value} onChange={(e) => onChange(e.target.value)} />
      <span className="text-content-muted">sec{suffix ? ` ${suffix}` : ""}</span>
    </label>
  );
}
