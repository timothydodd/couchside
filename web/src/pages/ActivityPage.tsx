import { Cpu, Image, MonitorPlay, Pause, RotateCcw, ScanSearch, Scissors, Search, Square, Trash2, Wand2, X, type LucideIcon } from "lucide-react";
import Link from "../components/Link";
import { EmptyState, ErrorNote, Meter, PageHeader, StatTile, StatusPill, type Tone, Loading } from "../components/ui";
import { api, useApi } from "../lib/api";
import { fmtAgo, fmtClock } from "../lib/format";
import type { Job, JobCounts, TranscodeSession } from "../lib/types";
import { attempt } from "../lib/notices";
import { confirmDialog } from "../lib/ask";

const KINDS: Record<string, { label: string; Icon: LucideIcon }> = {
  scan: { label: "Scan", Icon: ScanSearch },
  match: { label: "Match", Icon: Search },
  artwork: { label: "Artwork", Icon: Wand2 },
  still: { label: "Thumbnail", Icon: Image },
  optimize: { label: "Encode", Icon: Cpu },
  commercials: { label: "Commercials", Icon: Scissors },
  trickplay: { label: "Thumbnails", Icon: Image },
  intros: { label: "Intros", Icon: Search },
};

const STATUS: Record<Job["status"], { label: string; tone: Tone }> = {
  running: { label: "Running", tone: "info" },
  queued: { label: "Queued", tone: "muted" },
  done: { label: "Done", tone: "good" },
  failed: { label: "Failed", tone: "critical" },
};

/** Background work (scans, metadata, thumbnails, encodes) and live streams. */
export default function ActivityPage() {
  const { data, error, reload } = useApi<{ jobs: Job[]; counts: JobCounts }>("/api/jobs", { pollMs: 2500 });
  const c = data?.counts;

  const cancel = attempt("Couldn't cancel the job", async (id: number) => {
    await api(`/api/jobs/${id}/cancel`, { method: "POST" });
    await reload();
  });

  const retry = attempt("Couldn't retry the job", async (id: number) => {
    await api(`/api/jobs/${id}/retry`, { method: "POST" });
    await reload();
  });
  const clear = attempt("Couldn't clear finished jobs", async () => {
    if (!(await confirmDialog({ title: "Clear the finished jobs from the list?", action: "Clear" }))) return;
    await api("/api/jobs/clear", { method: "POST" });
    await reload();
  });
  const scanAll = attempt("Couldn't start the scans", async () => {
    await api("/api/libraries/scan", { method: "POST" });
    await reload();
  });

  return (
    <div>
      <PageHeader title="Activity" subtitle="Live streams, encodes, library scans and metadata lookups">
        <button className="btn-ghost" onClick={() => void scanAll()}>
          <ScanSearch size={15} /> Scan all libraries
        </button>
        <button className="btn-ghost" onClick={() => void clear()} disabled={!data?.jobs.some((j) => j.status === "done")}>
          <Trash2 size={15} /> Clear finished
        </button>
      </PageHeader>
      <div className="grid grid-cols-3 gap-3 gutter py-4">
        <StatTile label="Running" value={c?.running ?? "–"} sub={c?.current || undefined} />
        <StatTile label="Queued" value={c?.queued ?? "–"} />
        <StatTile label="Failed" value={c?.failed ?? "–"} tone={c?.failed ? "critical" : undefined} />
      </div>
      {error && (
        <div className="gutter pt-4">
          <ErrorNote>{error}</ErrorNote>
        </div>
      )}
      <Streams />
      <div className="gutter">
        <h2 className="row-title mb-3">Background jobs</h2>
        {!data ? (
          !error && <Loading />
        ) : data.jobs.length === 0 ? (
          <div className="card">
            <EmptyState title="Nothing going on">Scans run automatically on a schedule and whenever you add a library.</EmptyState>
          </div>
        ) : (
          <div className="card overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th>Task</th>
                  <th>Target</th>
                  <th>Status</th>
                  <th>When</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {data?.jobs.map((j) => {
                  const k = KINDS[j.kind] ?? { label: j.kind, Icon: Wand2 };
                  const s = STATUS[j.status];
                  return (
                    <tr key={j.id}>
                      <td>
                        <span className="inline-flex items-center gap-2 text-content-secondary">
                          <k.Icon size={14} className="text-content-muted" />
                          {k.label}
                        </span>
                      </td>
                      <td className="max-w-md">
                        <div className="truncate">{j.label.replace(/^\S+\s/, "")}</div>
                        {j.error && (
                          <div className="truncate text-xs text-critical" title={j.error}>
                            {j.error}
                          </div>
                        )}
                        {j.result && (
                          <div className={`line-clamp-2 whitespace-normal text-xs ${j.result.includes("Skipped") ? "text-warning" : "text-content-muted"}`} title={j.result}>
                            {j.result}
                          </div>
                        )}
                      </td>
                      <td className="min-w-40">
                        <StatusPill label={s.label} tone={s.tone} pulse={j.status === "running"} />
                        {j.attempts > 1 && <span className="ml-2 text-xs text-content-muted">try {j.attempts}</span>}
                        {j.status === "running" && j.progress != null && (
                          <div className="mt-1 flex items-center gap-2">
                            <Meter value={j.progress * 100} className="w-28" />
                            <span className="text-[11px] tabular-nums text-content-muted">{Math.round(j.progress * 100)}%</span>
                          </div>
                        )}
                      </td>
                      <td className="text-xs text-content-muted">{fmtAgo(j.finishedAt ?? j.startedAt ?? j.createdAt)}</td>
                      <td className="text-right">
                        {j.status === "failed" && (
                          <button className="btn-chip" onClick={() => void retry(j.id)}>
                            <RotateCcw size={12} /> Retry
                          </button>
                        )}
                        {(j.status === "queued" || (j.status === "running" && j.kind === "optimize")) && (
                          <button className="btn-chip hover:!border-critical hover:!text-critical" onClick={() => void cancel(j.id)}>
                            <X size={12} /> Cancel
                          </button>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

/** Live transcode sessions: who's watching what, and how hard the server is working. */
function Streams() {
  const { data, error } = useApi<{ hwaccel: string; maxSessions: number; sessions: TranscodeSession[] }>("/api/transcode", { pollMs: 2500 });
  if (!data)
    return error ? (
      <section className="gutter pb-6">
        <ErrorNote>Couldn't load what's streaming: {error}</ErrorNote>
      </section>
    ) : null;
  const engine = data.hwaccel === "none" ? "software (CPU)" : data.hwaccel.toUpperCase();
  return (
    <section className="gutter pb-6">
      <div className="mb-3 flex items-baseline justify-between">
        <h2 className="row-title">Streaming now</h2>
        <span className="text-xs text-content-muted">
          Encoder: {engine} · up to {data.maxSessions} at once
        </span>
      </div>
      {data.sessions.length === 0 ? (
        <div className="card px-4 py-3 text-xs text-content-muted">Nothing is being transcoded. Files that play directly don't show up here.</div>
      ) : (
        <div className="grid gap-3 md:grid-cols-2">
          {data.sessions.map((s) => (
            <div key={s.id} className="card flex items-center gap-3 px-4 py-3">
              <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-muted text-accent">
                <MonitorPlay size={17} />
              </div>
              <div className="min-w-0 flex-1">
                <Link to={`/play/${s.fileId}`} className="block truncate text-sm font-medium text-content hover:text-accent">
                  {s.title}
                </Link>
                <div className="mt-0.5 flex flex-wrap gap-x-2 text-xs text-content-muted">
                  <span>{s.mode === "remux" ? "Direct stream" : `Transcoding ${s.height}p`}</span>
                  {s.hdr && <span>HDR→SDR</span>}
                  {s.mode === "transcode" && <span>{s.hw && s.hw !== "none" ? s.hw.toUpperCase() : "CPU"}</span>}
                  <span>at {fmtClock(s.positionSec)}</span>
                  <span>{Math.round(s.aheadSec)}s buffered ahead</span>
                </div>
              </div>
              <span title={s.paused ? "Far enough ahead of the player; paused to save CPU" : s.running ? "Encoding" : "Idle"}>
                {s.paused ? <Pause size={15} className="text-content-muted" /> : s.running ? <StatusPill label="" tone="info" pulse /> : <Square size={13} className="text-content-muted" />}
              </span>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
