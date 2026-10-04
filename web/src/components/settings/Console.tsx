import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { ArrowDownToLine, Pause, Play } from "lucide-react";
import { SearchInput, Segmented } from "../ui";
import { api } from "../../lib/api";
import type { LogEntry } from "../../lib/types";

const KEEP = 2000; // lines held in the page, like the server
const LEVELS = [
  { id: "all", label: "All", min: 0 },
  { id: "warn", label: "Warnings", min: 2 },
  { id: "error", label: "Errors", min: 3 },
] as const;
type Level = (typeof LEVELS)[number]["id"];
const RANK: Record<string, number> = { DEBUG: 0, INFO: 1, WARN: 2, ERROR: 3 };

type Line = LogEntry | { seq: number; gap: true };

/** The server's log, following new lines every 2 seconds. */
export default function Console() {
  const [lines, setLines] = useState<Line[]>([]);
  const [paused, setPaused] = useState(false);
  const [level, setLevel] = useState<Level>("all");
  const [q, setQ] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [follow, setFollow] = useState(true);
  const last = useRef(0);
  const box = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (paused) return;
    let stop = false;
    let timer = 0;
    // One request at a time: the next is sent 2s after the last answered. Two
    // overlapping polls ask for the same lines and both add them.
    const poll = async () => {
      if (document.visibilityState !== "visible") return;
      try {
        const r = await api<{ entries: LogEntry[]; last: number; gap: boolean }>(`/api/system/logs?after=${last.current}`);
        if (stop) return;
        setError(null);
        const wasEmpty = last.current === 0;
        last.current = r.last;
        if (!r.entries.length) return;
        setLines((ls) => {
          const add: Line[] = r.gap && !wasEmpty ? [{ seq: -r.entries[0].seq, gap: true }, ...r.entries] : r.entries;
          const all = ls.concat(add);
          return all.length > KEEP ? all.slice(all.length - KEEP) : all;
        });
      } catch (e) {
        if (!stop) setError(e instanceof Error ? e.message : String(e));
      }
    };
    const loop = async () => {
      await poll();
      if (!stop) timer = window.setTimeout(() => void loop(), 2000);
    };
    void loop();
    return () => {
      stop = true;
      clearTimeout(timer);
    };
  }, [paused]);

  // Stay at the bottom while following; scrolling up to read stops it.
  useLayoutEffect(() => {
    if (follow && box.current) box.current.scrollTop = box.current.scrollHeight;
  }, [lines, follow, level, q]);

  const min = LEVELS.find((l) => l.id === level)!.min;
  const needle = q.trim().toLowerCase();
  const shown = lines.filter(
    (l) => "gap" in l || ((RANK[l.level] ?? 1) >= min && (!needle || `${l.msg} ${l.attrs}`.toLowerCase().includes(needle))),
  );

  return (
    <section className="card flex flex-col p-0">
      <div className="flex flex-wrap items-center gap-2 border-b border-border-light p-3">
        <Segmented label="Level" value={level} onChange={setLevel} options={LEVELS.map((l) => ({ id: l.id, label: l.label }))} />
        <SearchInput value={q} onChange={setQ} placeholder="Filter lines…" aria-label="Filter lines" className="min-w-40 flex-1" />
        <button className="btn-ghost" onClick={() => setPaused((p) => !p)} aria-pressed={paused}>
          {paused ? <Play size={14} /> : <Pause size={14} />} {paused ? "Resume" : "Pause"}
        </button>
      </div>
      <div className="relative">
        <div
          ref={box}
          className="console"
          role="log"
          aria-live="off"
          onScroll={(e) => {
            const el = e.currentTarget;
            setFollow(el.scrollHeight - el.scrollTop - el.clientHeight < 24);
          }}
        >
          {shown.length === 0 && <div className="py-8 text-center text-content-muted">{lines.length ? "No lines match." : "No log lines yet."}</div>}
          {shown.map((l) =>
            "gap" in l ? (
              <div key={l.seq} className="console-gap">
                Some lines were dropped here (the server keeps the last 2,000, or it restarted)
              </div>
            ) : (
              <div key={l.seq} className={`console-line console-${l.level.toLowerCase()}`}>
                <span className="console-time">{new Date(l.t).toLocaleTimeString(undefined, { hour12: false })}</span>
                <span className="console-level">{l.level}</span>
                <span className="console-msg">{l.msg}</span>
                {l.attrs && <span className="console-attrs"> {l.attrs}</span>}
              </div>
            ),
          )}
        </div>
        {!follow && (
          <button className="btn-ghost absolute bottom-3 right-4 bg-raised !text-xs shadow-md" onClick={() => setFollow(true)}>
            <ArrowDownToLine size={13} /> Latest
          </button>
        )}
      </div>
      <div className="flex items-center justify-between gap-3 border-t border-border-light px-3 py-2 text-xs text-content-muted">
        <span>
          {shown.filter((l) => !("gap" in l)).length.toLocaleString()} of {lines.filter((l) => !("gap" in l)).length.toLocaleString()} lines
          {paused && " · paused"}
        </span>
        {error ? <span className="text-critical">Couldn't load the log: {error}</span> : <span>The full log is the server's standard output.</span>}
      </div>
    </section>
  );
}
