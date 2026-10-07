import { useRef, useState } from "react";
import { AlertTriangle, CircleDot, Play, Repeat, Square, X } from "lucide-react";
import SeriesForm from "./SeriesForm";
import { useCanRecord } from "../../stores/auth";
import Link from "../Link";
import { ApiError, api } from "../../lib/api";
import { fmtSlot } from "../../lib/format";
import type { Program, TvChannel } from "../../lib/types";
import { useDialog } from "../../lib/dialog";
import { confirmDialog } from "../../lib/ask";

/** Program details with Watch and Record actions. */
export default function ProgramDialog({
  program,
  channel,
  onClose,
  onChange,
}: {
  program: Program;
  channel?: TvChannel;
  onClose: () => void;
  onChange: () => void;
}) {
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ tone: "warning" | "critical"; text: string } | null>(null);
  const [series, setSeries] = useState(false);
  const now = Date.now() / 1000;
  const airing = program.startAt <= now && program.endAt > now;
  const ended = program.endAt <= now;
  const drm = channel?.drm;
  // Couchside's own channels play from the library: nothing to record.
  const canRecord = useCanRecord() && !channel?.virtual;

  const dialog = useRef<HTMLDivElement>(null);
  useDialog(dialog, onClose);

  const record = async () => {
    setBusy(true);
    setMsg(null);
    try {
      const r = await api<{ conflict: boolean; overlapping: number; tuners: number }>("/api/dvr/recordings", {
        method: "POST",
        json: { programId: program.id },
      });
      if (r.conflict)
        setMsg({
          tone: "warning",
          text: `Scheduled, but ${r.overlapping} other recordings overlap and there are only ${r.tuners} tuners. Plex or live viewers can also take tuners.`,
        });
      onChange();
    } catch (e) {
      setMsg({ tone: "critical", text: e instanceof ApiError ? e.message : String(e) });
    } finally {
      setBusy(false);
    }
  };

  const cancel = async () => {
    if (!program.recordingId) return;
    if (program.recordingStatus === "recording" && !(await confirmDialog({ title: `Stop recording "${program.title}"?`, body: "What's been recorded so far is kept.", action: "Stop recording", danger: true }))) return;
    setBusy(true);
    setMsg(null);
    try {
      await api(`/api/dvr/recordings/${program.recordingId}/cancel`, { method: "POST" });
      onChange();
    } catch (e) {
      setMsg({ tone: "critical", text: e instanceof ApiError ? e.message : String(e) });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-backdrop/55 p-4" onClick={onClose}>
      <div className="card relative max-h-[92vh] w-full max-w-lg overflow-y-auto shadow-[var(--shadow-md)]" onClick={(e) => e.stopPropagation()} role="dialog" aria-modal="true" aria-label={program.title} ref={dialog}>
        {program.imageUrl && (
          <div className="relative aspect-video bg-raised">
            <img src={program.imageUrl} alt="" className="h-full w-full object-cover" />
            <div className="absolute inset-0 bg-gradient-to-t from-[var(--bg-surface)] to-transparent" />
          </div>
        )}
        <button onClick={onClose} className="btn-quiet absolute right-2 top-2 !bg-surface/70 backdrop-blur" aria-label="Close">
          <X size={16} />
        </button>
        <div className={`p-5 ${program.imageUrl ? "-mt-10 relative" : ""}`}>
          <div className="flex flex-wrap items-center gap-1.5">
            {airing && <span className="tint-info badge">On now</span>}
            {program.isNew && <span className="tint-good badge">New</span>}
            {program.recordingStatus === "recording" && (
              <span className="tint-critical badge">
                <span className="rec-dot animate-pulse" /> Recording
              </span>
            )}
            {program.recordingStatus === "scheduled" && (
              <span className="tint-critical badge">
                <span className="rec-dot" /> Will record
              </span>
            )}
            {program.ruleId && (
              <span className="tint-info badge">
                <Repeat size={11} /> Series
              </span>
            )}
            {program.categories.slice(0, 3).map((c) => (
              <span key={c} className="chip">
                {c}
              </span>
            ))}
          </div>
          <h2 className="mt-2 text-xl font-bold text-content">{program.title}</h2>
          {(program.episodeNum || program.episodeTitle) && (
            <div className="mt-0.5 text-sm text-content-secondary">{[program.episodeNum, program.episodeTitle].filter(Boolean).join(" · ")}</div>
          )}
          <div className="mt-1 text-xs text-content-muted">
            {fmtSlot(program.startAt, program.endAt)}
            {channel && ` · ${channel.number} ${channel.name}`}
          </div>
          {program.synopsis && <p className="mt-3 text-sm leading-relaxed text-content-secondary">{program.synopsis}</p>}

          {msg && (
            <div className={`tint-${msg.tone} mt-3 flex items-start gap-2 rounded-md px-3 py-2 text-xs`}>
              <AlertTriangle size={14} className="mt-px shrink-0" />
              {msg.text}
            </div>
          )}
          {drm && <div className="mt-3 text-xs text-content-muted">This channel is copy-protected (ATSC 3.0), so it can't be watched or recorded here.</div>}

          <div className="mt-5 flex flex-wrap gap-2">
            {program.recordingStatus === "recording" && program.recordingId && (
              <Link to={`/recording/${program.recordingId}`} className="btn-primary">
                <Play size={15} className="fill-current" /> Watch from start
              </Link>
            )}
            {airing && !drm && (
              <Link to={`/watch/${program.channel}`} className={program.recordingStatus === "recording" ? "btn-ghost" : "btn-primary"}>
                <Play size={15} className="fill-current" /> Watch live
              </Link>
            )}
            {canRecord && !ended && !drm && !program.recordingStatus && (
              <button className="btn-ghost hover:!border-critical" disabled={busy} onClick={() => void record()}>
                <CircleDot size={15} className="text-critical" /> Record
              </button>
            )}
            {canRecord && program.recordingStatus === "scheduled" && (
              <button className="btn-ghost" disabled={busy} onClick={() => void cancel()}>
                <X size={15} /> Don't record
              </button>
            )}
            {canRecord && program.recordingStatus === "recording" && (
              <button className="btn-ghost hover:!border-critical hover:!text-critical" disabled={busy} onClick={() => void cancel()}>
                <Square size={13} className="fill-current" /> Stop recording
              </button>
            )}
            {program.recordingStatus === "completed" && (
              <Link to="/livetv/recordings" className="btn-ghost">
                Recorded
              </Link>
            )}
            {canRecord && program.seriesId && !drm && !series && (
              <button className="btn-ghost" onClick={() => setSeries(true)}>
                <Repeat size={15} /> {program.ruleId ? "Series settings" : "Record series"}
              </button>
            )}
          </div>
          {series && (
            <SeriesForm
              program={program}
              channel={channel}
              onCancel={() => setSeries(false)}
              onDone={() => {
                setSeries(false);
                onChange();
              }}
            />
          )}
        </div>
      </div>
    </div>
  );
}
