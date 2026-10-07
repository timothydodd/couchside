import { CalendarClock, Film, Pause, Play, RadioTower, Repeat, RotateCcw, Square, Trash2, X } from "lucide-react";
import Link from "../Link";
import { EmptyState, ErrorNote, Loading, Meter } from "../ui";
import { api, useApi } from "../../lib/api";
import { fmtBytes, fmtDay, fmtSlot, fmtTime } from "../../lib/format";
import type { Recording, RuleMode, SeriesRule } from "../../lib/types";
import { KEEP_OPTIONS, MODE_TEXT, describeSummary, keepLabel } from "./rules";
import { useOwnerCheck } from "../../stores/auth";
import { attempt } from "../../lib/notices";
import { confirmDialog, type ConfirmOptions } from "../../lib/ask";

/** DVR: recording now, upcoming, recorded and failed. */
export default function Recordings() {
  const { data, error, loading, reload } = useApi<Recording[]>("/api/dvr/recordings", { pollMs: 5000 });
  const { data: rules, reload: reloadRules } = useApi<SeriesRule[]>("/api/dvr/rules", { pollMs: 15000 });
  // Who may change what: admins anything, people allowed to record their own.
  const may = useOwnerCheck();

  const act = attempt("Couldn't change the recording", async (path: string, method: "POST" | "DELETE", ask?: ConfirmOptions) => {
    if (ask && !(await confirmDialog({ danger: true, ...ask }))) return;
    await api(path, { method });
    await reload();
  });

  if (loading && !data)
    return (
      <Loading fill />
    );

  const recs = data ?? [];
  const live = recs.filter((r) => r.status === "recording");
  const upcoming = recs.filter((r) => r.status === "scheduled");
  const done = recs.filter((r) => r.status === "completed");
  const failed = recs.filter((r) => r.status === "failed");
  const now = Date.now() / 1000;

  const label = (r: Recording) => [r.episodeNum, r.episodeTitle].filter(Boolean).join(" · ");

  const updateRule = attempt("Couldn't update the series", async (r: SeriesRule, patch: Partial<SeriesRule>) => {
    const next = { ...r, ...patch };
    await api(`/api/dvr/rules/${r.id}`, {
      method: "PUT",
      json: { mode: next.mode, channel: next.channel, mediaItemId: next.mediaItemId, keepLast: next.keepLast, enabled: next.enabled },
    });
    await Promise.all([reloadRules(), reload()]);
  });
  const deleteRule = attempt("Couldn't stop recording the series", async (r: SeriesRule) => {
    if (!(await confirmDialog({ title: `Stop recording "${r.title}" as a series?`, body: "Upcoming recordings from it are cancelled; finished ones are kept.", action: "Stop recording", danger: true }))) return;
    await api(`/api/dvr/rules/${r.id}`, { method: "DELETE" });
    await Promise.all([reloadRules(), reload()]);
  });

  return (
    <div className="min-h-0 flex-1 overflow-auto gutter pb-8">
      {error && (
        <div className="pt-3">
          <ErrorNote>{error}</ErrorNote>
        </div>
      )}
      {!!rules?.length && (
        <section className="pt-4">
          <h2 className="row-title mb-3">Series</h2>
          <div className="grid gap-3 lg:grid-cols-2">
            {rules.map((r) => {
              let summary = "";
              try {
                summary = r.lastSummary ? describeSummary(JSON.parse(r.lastSummary)) : "";
              } catch {
                /* ignore */
              }
              return (
                <div key={r.id} className={`card flex gap-3 p-3 ${r.enabled ? "" : "opacity-60"}`}>
                  <div className="h-16 w-28 shrink-0 overflow-hidden rounded-md bg-raised">
                    {r.imageUrl ? <img src={r.imageUrl} alt="" loading="lazy" className="h-full w-full object-cover" /> : <div className="poster-placeholder h-full w-full" />}
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-1.5">
                      <Repeat size={13} className="shrink-0 text-accent" />
                      <span className="truncate text-sm font-semibold text-content">{r.title}</span>
                      {!r.enabled && <span className="tint-muted badge">Paused</span>}
                    </div>
                    <div className="mt-0.5 truncate text-xs text-content-muted">
                      {r.channel ? `Only ${r.channel}` : "Any channel"}
                      {r.mode === "missing" && (r.libraryTitle ? ` · compared with ${r.libraryTitle}` : " · not compared with the library")}
                    </div>
                    <div className="mt-0.5 text-xs text-content-secondary">
                      {r.scheduled ? `${r.scheduled} upcoming${r.nextAt ? `, next ${fmtDay(r.nextAt)} ${fmtTime(r.nextAt)}` : ""}` : "Nothing upcoming"} · {r.recorded} recorded
                    </div>
                    {summary && <div className="mt-1 line-clamp-2 text-[11px] text-content-muted">{summary}</div>}
                    {may(r.ownerId) && <div className="mt-2 flex flex-wrap items-center gap-1.5">
                      <select className="field !py-0.5 !text-xs" value={r.mode} onChange={(e) => void updateRule(r, { mode: e.target.value as RuleMode })} aria-label="What to record">
                        {(Object.keys(MODE_TEXT) as RuleMode[]).map((m) => (
                          <option key={m} value={m}>
                            {MODE_TEXT[m].label}
                          </option>
                        ))}
                      </select>
                      <select className="field !py-0.5 !text-xs" value={r.keepLast} onChange={(e) => void updateRule(r, { keepLast: Number(e.target.value) })} aria-label="Keep">
                        {KEEP_OPTIONS.map((n) => (
                          <option key={n} value={n}>
                            {keepLabel(n)}
                          </option>
                        ))}
                      </select>
                      <button className="btn-chip" onClick={() => void updateRule(r, { enabled: !r.enabled })}>
                        {r.enabled ? <Pause size={11} /> : <Play size={11} />} {r.enabled ? "Pause" : "Resume"}
                      </button>
                      <button className="btn-chip hover:!border-critical hover:!text-critical" onClick={() => void deleteRule(r)}>
                        <Trash2 size={11} /> Stop series
                      </button>
                    </div>}
                  </div>
                </div>
              );
            })}
          </div>
        </section>
      )}

      {recs.length === 0 && !rules?.length && (
        <EmptyState icon={<CalendarClock size={36} strokeWidth={1.5} />} title="Nothing recorded or scheduled">
          Open the guide, pick a show and choose Record.
        </EmptyState>
      )}

      {live.length > 0 && (
        <section className="pt-4">
          <h2 className="row-title mb-3">Recording now</h2>
          <div className="grid gap-3 md:grid-cols-2">
            {live.map((r) => {
              const pct = Math.min(100, ((now - r.startAt) / (r.endAt - r.startAt)) * 100);
              return (
                <div key={r.id} className="card flex flex-wrap items-center gap-3 border-critical/40 p-3">
                  <span className="rec-dot animate-pulse" />
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium text-content">{r.title}</div>
                    <div className="truncate text-xs text-content-muted">
                      {r.channel} {r.channelName} · until {fmtTime(r.endAt + r.padAfter)}
                      {label(r) && ` · ${label(r)}`}
                    </div>
                    <Meter value={pct} tone="critical" className="mt-1.5 !h-1" />
                    {r.error && <div className="mt-1 truncate text-xs text-warning">{r.error}</div>}
                  </div>
                  {/* On phones the buttons take a row of their own under the text. */}
                  <div className="flex w-full flex-wrap gap-2 sm:ml-auto sm:w-auto">
                    <Link to={`/recording/${r.id}`} className="btn-primary" title="Play this recording from the beginning">
                      <Play size={13} className="fill-current" /> From start
                    </Link>
                    <Link to={`/watch/${r.channel}`} className="btn-ghost" title="Jump to the live broadcast">
                      <RadioTower size={14} /> Live
                    </Link>
                    {may(r.ownerId) && (
                      <button className="btn-ghost hover:!border-critical hover:!text-critical" onClick={() => void act(`/api/dvr/recordings/${r.id}/cancel`, "POST", { title: `Stop recording "${r.title}"?`, body: "What's been recorded so far is kept.", action: "Stop recording" })}>
                        <Square size={12} className="fill-current" /> Stop
                      </button>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        </section>
      )}

      {upcoming.length > 0 && (
        <section className="pt-6">
          <h2 className="row-title mb-3">Upcoming</h2>
          <div className="card overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th>When</th>
                  <th>Program</th>
                  <th>Channel</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {upcoming.map((r) => (
                  <tr key={r.id}>
                    <td className="tabular-nums text-content-secondary">{fmtSlot(r.startAt, r.endAt)}</td>
                    <td className="max-w-md">
                      <div className="flex items-center gap-1.5 truncate font-medium text-content">
                        {r.ruleId && <Repeat size={12} className="shrink-0 text-accent" aria-label="From a series rule" />}
                        {r.title}
                      </div>
                      {label(r) && <div className="truncate text-xs text-content-muted">{label(r)}</div>}
                      {r.error && <div className="truncate text-xs text-warning">{r.error}</div>}
                    </td>
                    <td className="text-content-secondary">
                      {r.channel} {r.channelName}
                    </td>
                    <td className="text-right">
                      {may(r.ownerId) && (
                        <button className="btn-chip" onClick={() => void act(`/api/dvr/recordings/${r.id}/cancel`, "POST")}>
                          <X size={12} /> Cancel
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {done.length > 0 && (
        <section className="pt-6">
          <h2 className="row-title mb-3">Recorded</h2>
          <div className="card overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th>Program</th>
                  <th>Aired</th>
                  <th className="text-right">Size</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {done.map((r) => (
                  <tr key={r.id}>
                    <td className="max-w-md">
                      <div className="truncate font-medium text-content">{r.title}</div>
                      {label(r) && <div className="truncate text-xs text-content-muted">{label(r)}</div>}
                      {r.error && <div className="truncate text-xs text-warning">{r.error}</div>}
                    </td>
                    <td className="whitespace-nowrap text-content-secondary">
                      {fmtDay(r.startAt)} {fmtTime(r.startAt)} · {r.channelName || r.channel}
                    </td>
                    <td className="text-right tabular-nums text-content-secondary">{fmtBytes(r.size)}</td>
                    <td className="whitespace-nowrap text-right">
                      {r.fileId ? (
                        <Link to={`/play/${r.fileId}`} className="btn-chip mr-1.5">
                          <Play size={11} className="fill-current" /> Play
                        </Link>
                      ) : (
                        <span className="mr-2 inline-flex items-center gap-1 text-xs text-content-muted" title="Waiting for the library scan">
                          <Film size={11} /> Adding…
                        </span>
                      )}
                      {may(r.ownerId) && (
                        <button
                          className="btn-chip hover:!border-critical hover:!text-critical"
                          onClick={() => void act(`/api/dvr/recordings/${r.id}`, "DELETE", { title: `Delete the recording of "${r.title}"?`, body: "The file is removed from disk.", action: "Delete" })}
                        >
                          <Trash2 size={11} /> Delete
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {failed.length > 0 && (
        <section className="pt-6">
          <h2 className="row-title mb-3">Didn't record</h2>
          <div className="card overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th>Recording</th>
                  <th>When</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {failed.map((r) => (
                  <tr key={r.id}>
                    <td className="max-w-md">
                      <div className="truncate font-medium text-content">{r.title}</div>
                      <div className="truncate text-xs text-critical" title={r.error}>
                        {r.error}
                      </div>
                    </td>
                    <td className="whitespace-nowrap text-content-secondary">
                      {fmtDay(r.startAt)} {fmtTime(r.startAt)} · {r.channelName || r.channel}
                    </td>
                    <td className="text-right">
                      {may(r.ownerId) && (
                        <span className="inline-flex gap-2">
                          {r.recoverable && (
                            <button
                              className="btn-chip"
                              title="What was recorded is still on disk in pieces. Join them into the recording."
                              onClick={() => void act(`/api/dvr/recordings/${r.id}/recover`, "POST")}
                            >
                              <RotateCcw size={11} /> Recover
                            </button>
                          )}
                          <button className="btn-chip" onClick={() => void act(`/api/dvr/recordings/${r.id}`, "DELETE")}>
                            <X size={11} /> Dismiss
                          </button>
                        </span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
    </div>
  );
}
