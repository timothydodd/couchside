import { useCallback, useEffect, useRef, useState, type ReactNode, type RefObject, type VideoHTMLAttributes } from "react";
import {
  AlertTriangle, ArrowLeft, ChevronsRight, Maximize, Minimize, Pause, Play, Radio, RotateCcw, RotateCw, Settings2, SkipBack, Volume1, Volume2, VolumeX,
} from "lucide-react";
import SeekBar from "./SeekBar";
import SettingsMenu, { type SettingSection } from "./SettingsMenu";
import { useMediaState } from "./useMediaState";
import { useBreakSkip } from "./useBreakSkip";
import { fmtClock, fmtTime } from "../../lib/format";
import type { BreakMode, Segment } from "../../lib/types";

/**
 * How the timeline behaves:
 * - vod: a file with a fixed duration. With parts, the file is one part of a
 *   movie and the bar spans the whole movie (see PartsTimeline).
 * - live: a growing live stream you can rewind within; "Live" jumps to the edge.
 *   With guide programs, the bar spans the program being watched in clock
 *   time (9:00–10:00 at 9:30 sits in the middle): before the stream started
 *   is shaded, not-yet-aired is hatched.
 * - recording: a recording in progress, drawn over the whole program
 *   (startAt..endAt as wall-clock unix seconds, video time 0 = startAt).
 */
export type Timeline =
  | { kind: "vod"; parts?: PartsTimeline }
  | { kind: "live"; programs?: { startAt: number; endAt: number }[] }
  | { kind: "recording"; startAt: number; endAt: number };

/**
 * A movie split across files. The playing file starts at offset on the
 * movie's clock; seek is called with a movie time outside it.
 */
export interface PartsTimeline {
  offset: number;
  total: number;
  starts: number[]; // where each later part begins, marked on the bar
  seek: (t: number) => void;
}

export interface PlayerFrameProps {
  videoRef: RefObject<HTMLVideoElement | null>;
  title: string;
  subtitle?: ReactNode;
  badge?: ReactNode;
  logo?: string;
  topActions?: ReactNode;
  timeline: Timeline;
  /** Position of the live edge (hls.liveSyncPosition); defaults to a few seconds before the seekable end. */
  liveEdge?: () => number | null;
  settings: SettingSection[];
  /** Commercial breaks to mark and skip (vod only). */
  breaks?: Segment[];
  breakMode?: BreakMode;
  onBack: () => void;
  onKey?: (e: KeyboardEvent) => boolean; // return true when handled
  loading?: string | null;
  notice?: string | null;
  error?: { title: string; message: string; actions?: ReactNode } | null;
  videoProps?: VideoHTMLAttributes<HTMLVideoElement>;
  children?: ReactNode; // e.g. <track> elements
}

const LIVE_SLACK = 12; // seconds behind the edge that still counts as "live"

/** The player shell shared by movies/episodes, live TV and recordings. */
export default function PlayerFrame(p: PlayerFrameProps) {
  const { videoRef, timeline } = p;
  const root = useRef<HTMLDivElement>(null);
  const st = useMediaState(videoRef);
  const brk = useBreakSkip(videoRef, st.time, timeline.kind === "vod" ? p.breaks : undefined, p.breakMode ?? "off");
  const [chrome, setChrome] = useState(true);
  const [menu, setMenu] = useState(false);
  const [full, setFull] = useState(false);
  const [volOpen, setVolOpen] = useState(false);
  const hideTimer = useRef<ReturnType<typeof setTimeout>>(undefined);

  const v = () => videoRef.current;
  const isLive = timeline.kind !== "vod";
  const parts = timeline.kind === "vod" ? timeline.parts : undefined;
  const off = parts?.offset ?? 0;
  const edge = useCallback(() => {
    const e = p.liveEdge?.();
    return e ?? Math.max(st.seekStart, st.seekEnd - 3);
  }, [p, st.seekEnd, st.seekStart]);
  const behind = isLive ? Math.max(0, st.seekEnd - st.time) : 0;
  const atLive = isLive && behind < LIVE_SLACK;

  // --- chrome auto-hide -----------------------------------------------------------
  const poke = useCallback(() => {
    setChrome(true);
    clearTimeout(hideTimer.current);
    hideTimer.current = setTimeout(() => {
      if (!videoRef.current?.paused) setChrome(false);
    }, 3000);
  }, [videoRef]);
  useEffect(() => {
    poke();
    return () => clearTimeout(hideTimer.current);
  }, [poke]);
  useEffect(() => {
    if (st.paused) setChrome(true);
  }, [st.paused]);

  // --- actions ----------------------------------------------------------------------
  const toggle = useCallback(() => {
    const el = v();
    if (!el) return;
    if (el.paused) void el.play().catch(() => {});
    else el.pause();
  }, []); // eslint-disable-line react-hooks/exhaustive-deps
  const seekTo = useCallback((t: number) => {
    const el = v();
    if (el) el.currentTime = t;
  }, []); // eslint-disable-line react-hooks/exhaustive-deps
  // A time on the whole bar: outside the playing part, another part takes over.
  const seekBar = useCallback(
    (t: number) => {
      const el = v();
      if (!el) return;
      const end = isFinite(el.duration) ? el.duration : Infinity;
      if (parts && (t < off || t - off >= end - 0.5)) parts.seek(Math.max(0, Math.min(parts.total, t)));
      else seekTo(t - off);
    },
    [parts, off, seekTo], // eslint-disable-line react-hooks/exhaustive-deps
  );
  const skip = useCallback(
    (d: number) => {
      const el = v();
      if (!el) return;
      if (parts && (el.currentTime + d < 0 || (isFinite(el.duration) && el.currentTime + d >= el.duration))) {
        seekBar(off + el.currentTime + d);
        return;
      }
      const hi = isLive ? st.seekEnd : isFinite(el.duration) ? el.duration : el.currentTime + d;
      const t = Math.max(isLive ? st.seekStart : 0, Math.min(hi, el.currentTime + d));
      brk.allow(t);
      el.currentTime = t;
    },
    [isLive, st.seekEnd, st.seekStart, brk.allow, parts, off, seekBar], // eslint-disable-line react-hooks/exhaustive-deps
  );
  const goLive = () => {
    seekTo(edge());
    void v()?.play().catch(() => {});
  };
  const setVolume = (x: number) => {
    const el = v();
    if (!el) return;
    el.volume = Math.max(0, Math.min(1, x));
    el.muted = el.volume === 0;
    try {
      localStorage.setItem("couchside:volume", String(el.volume));
    } catch {
      /* ignore */
    }
  };
  const toggleFull = useCallback(() => {
    if (document.fullscreenElement) void document.exitFullscreen();
    else void root.current?.requestFullscreen().catch(() => {});
  }, []);

  useEffect(() => {
    const onFs = () => setFull(!!document.fullscreenElement);
    document.addEventListener("fullscreenchange", onFs);
    return () => document.removeEventListener("fullscreenchange", onFs);
  }, []);

  // restore volume once
  useEffect(() => {
    const el = v();
    try {
      const saved = Number(localStorage.getItem("couchside:volume"));
      if (el && saved > 0 && saved <= 1) el.volume = saved;
    } catch {
      /* ignore */
    }
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // --- keyboard ---------------------------------------------------------------------
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement)?.closest("input, select, textarea")) return;
      poke();
      if (p.onKey?.(e)) return;
      const el = v();
      switch (e.key) {
        case " ":
        case "k":
          e.preventDefault();
          toggle();
          break;
        case "ArrowLeft":
        case "j":
          e.preventDefault();
          skip(-10);
          break;
        case "ArrowRight":
        case "l":
          e.preventDefault();
          skip(e.key === "l" ? 30 : 10);
          break;
        case "ArrowUp":
          e.preventDefault();
          if (el) setVolume(el.volume + 0.1);
          break;
        case "ArrowDown":
          e.preventDefault();
          if (el) setVolume(el.volume - 0.1);
          break;
        case "m":
          if (el) el.muted = !el.muted;
          break;
        case "f":
          toggleFull();
          break;
        case "s":
          brk.skip();
          break;
        case "Escape":
          if (menu) setMenu(false);
          else if (!document.fullscreenElement) p.onBack();
          break;
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  // --- timeline math ------------------------------------------------------------------
  let min = 0;
  let max = isFinite(st.duration) ? st.duration : st.seekEnd;
  let recordedEnd: number | undefined;
  let label = (t: number) => fmtClock(t);
  let availableStart: number | undefined;
  // Live with a guide: map video time to the clock (the edge is now) and span the
  // program the playhead is in.
  const clockAt = (t: number) => Date.now() / 1000 - (st.seekEnd - t);
  const program = timeline.kind === "live" ? timeline.programs?.find((g) => clockAt(st.time) >= g.startAt && clockAt(st.time) < g.endAt) : undefined;
  if (parts) {
    max = parts.total;
    label = (t: number) => `${fmtClock(t)} · Part ${parts.starts.filter((s) => s <= t).length + 1}`;
  } else if (program) {
    const nowSec = Date.now() / 1000;
    min = st.seekEnd - (nowSec - program.startAt);
    max = st.seekEnd + (program.endAt - nowSec);
    availableStart = Math.max(min, st.seekStart);
    recordedEnd = st.seekEnd;
    label = (t: number) => fmtTime(clockAt(t));
  } else if (timeline.kind === "live") {
    min = st.seekStart;
    max = Math.max(st.seekEnd, st.seekStart + 1);
    label = (t: number) => fmtTime(Date.now() / 1000 - (st.seekEnd - t));
  } else if (timeline.kind === "recording") {
    min = 0;
    max = Math.max(timeline.endAt - timeline.startAt, st.seekEnd);
    recordedEnd = st.seekEnd;
    label = (t: number) => fmtTime(timeline.startAt + t);
  }

  const timeText =
    parts
      ? `${fmtClock(off + st.time)} / ${fmtClock(parts.total)}`
      : timeline.kind === "vod"
      ? `${fmtClock(st.time)} / ${isFinite(st.duration) ? fmtClock(st.duration) : "–"}`
      : timeline.kind === "recording"
        ? `${fmtTime(timeline.startAt + st.time)} · ${fmtClock(st.time)} of ${fmtClock(timeline.endAt - timeline.startAt)}`
        : program
          ? `${fmtTime(clockAt(st.time))} · ${fmtClock(Math.max(0, clockAt(st.time) - program.startAt))} of ${fmtClock(program.endAt - program.startAt)}${atLive ? "" : ` · −${fmtClock(behind)}`}`
          : atLive
            ? fmtTime(Date.now() / 1000)
            : `${fmtTime(Date.now() / 1000 - behind)} · −${fmtClock(behind)}`;

  const VolIcon = st.muted || st.volume === 0 ? VolumeX : st.volume < 0.5 ? Volume1 : Volume2;
  const show = chrome || menu || !!p.error;

  return (
    <div
      ref={root}
      data-theme="dark"
      className="fixed inset-0 z-50 select-none bg-[var(--player-bg)] text-white"
      style={{ cursor: show ? "auto" : "none" }}
      onMouseMove={poke}
    >
      <video
        ref={videoRef}
        className="h-full w-full"
        autoPlay
        playsInline
        onClick={() => (menu ? setMenu(false) : toggle())}
        onDoubleClick={toggleFull}
        {...p.videoProps}
      >
        {p.children}
      </video>

      {/* center: loading / paused */}
      {(p.loading || (st.waiting && !st.paused)) && !p.error && (
        <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center gap-3">
          <div className="h-10 w-10 animate-spin rounded-full border-2 border-white/20 border-t-white/90" />
          {p.loading && <div className="text-sm text-white/80">{p.loading}</div>}
        </div>
      )}
      {st.paused && !p.loading && !p.error && st.width > 0 && (
        <button
          className="absolute left-1/2 top-1/2 flex h-16 w-16 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full bg-black/55 text-white backdrop-blur transition-transform hover:scale-105"
          onClick={toggle}
          aria-label="Play"
        >
          <Play size={28} className="translate-x-0.5 fill-current" />
        </button>
      )}

      {/* top bar */}
      <div
        className={`absolute inset-x-0 top-0 flex items-start gap-3 bg-gradient-to-b from-black/85 via-black/45 to-transparent px-5 pb-14 pt-4 transition-opacity duration-300 ${
          show ? "opacity-100" : "pointer-events-none opacity-0"
        }`}
      >
        <button onClick={p.onBack} className="mt-0.5 rounded-md p-1.5 text-white/85 hover:bg-white/10 hover:text-white" aria-label="Back">
          <ArrowLeft size={19} />
        </button>
        {p.logo && <img src={p.logo} alt="" className="channel-logo mt-0.5 max-h-9 max-w-16" />}
        <div className="min-w-0 flex-1">
          {p.badge && <div className="mb-0.5 flex items-center gap-2 text-xs text-white/70">{p.badge}</div>}
          <div className="truncate text-lg font-semibold leading-tight">{p.title}</div>
          {p.subtitle && <div className="truncate text-xs text-white/65">{p.subtitle}</div>}
        </div>
        <div className="flex items-center gap-1">{p.topActions}</div>
      </div>

      {p.notice && !p.error && (
        <div className="tint-info pointer-events-none absolute left-1/2 top-20 max-w-md -translate-x-1/2 rounded-md px-3 py-2 text-center text-xs backdrop-blur">
          {p.notice}
        </div>
      )}

      {(brk.current || brk.skipped) && !p.error && (
        <div className="absolute bottom-28 right-5 z-20" onClick={(e) => e.stopPropagation()}>
          {brk.current ? (
            <button className="player-pill" onClick={brk.skip} title="Skip commercial (S)">
              Skip commercial <ChevronsRight size={16} />
            </button>
          ) : (
            <button className="player-pill" onClick={brk.watch}>
              Skipped a {fmtClock(brk.skipped!.end - brk.skipped!.start)} commercial break · <span className="underline">Watch it</span>
            </button>
          )}
        </div>
      )}

      {/* bottom controls */}
      <div
        className={`absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/90 via-black/55 to-transparent px-5 pb-3 pt-16 transition-opacity duration-300 ${
          show ? "opacity-100" : "pointer-events-none opacity-0"
        }`}
        onClick={(e) => e.stopPropagation()}
      >
        <SeekBar
          min={min}
          max={max}
          value={off + st.time}
          bufferedEnd={off + st.bufferedEnd}
          recordedEnd={recordedEnd}
          availableStart={availableStart}
          breaks={off ? brk.breaks?.map((b) => ({ start: b.start + off, end: b.end + off })) : brk.breaks}
          marks={parts?.starts}
          label={label}
          onSeek={(t) => {
            brk.allow(t - off);
            seekBar(t);
          }}
        />
        <div className="mt-1 flex items-center gap-1">
          <CtlButton label={st.paused ? "Play (Space)" : "Pause (Space)"} onClick={toggle}>
            {st.paused ? <Play size={20} className="fill-current" /> : <Pause size={20} className="fill-current" />}
          </CtlButton>
          <CtlButton label="Back 10 seconds (←)" onClick={() => skip(-10)}>
            <RotateCcw size={18} />
          </CtlButton>
          <CtlButton label="Forward 30 seconds (L)" onClick={() => skip(30)} disabled={atLive}>
            <RotateCw size={18} />
          </CtlButton>
          <div className="relative flex items-center" onMouseEnter={() => setVolOpen(true)} onMouseLeave={() => setVolOpen(false)}>
            <CtlButton label="Mute (M)" onClick={() => { const el = v(); if (el) el.muted = !el.muted; }}>
              <VolIcon size={19} />
            </CtlButton>
            <input
              type="range"
              min={0}
              max={1}
              step={0.02}
              value={st.muted ? 0 : st.volume}
              onChange={(e) => setVolume(Number(e.target.value))}
              className={`accent-brand h-1 cursor-pointer transition-[width,opacity] duration-200 ${volOpen ? "w-24 opacity-100" : "w-0 opacity-0"}`}
              aria-label="Volume"
            />
          </div>
          <span className="ml-2 whitespace-nowrap text-xs tabular-nums text-white/80">{timeText}</span>
          <div className="flex-1" />
          {timeline.kind === "recording" && (
            <CtlButton label="Start over" onClick={() => seekTo(0)} wide>
              <SkipBack size={15} /> <span className="text-xs">Start over</span>
            </CtlButton>
          )}
          {isLive && (
            <button
              onClick={goLive}
              className={`flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-bold tracking-wide ${
                atLive ? "bg-critical text-white" : "bg-white/10 text-white/80 hover:bg-white/20 hover:text-white"
              }`}
              title={atLive ? "You're watching live" : "Jump to live"}
            >
              <Radio size={13} /> {atLive ? "LIVE" : "GO LIVE"}
            </button>
          )}
          <CtlButton label="Settings" onClick={() => setMenu((m) => !m)} active={menu}>
            <Settings2 size={19} />
          </CtlButton>
          <CtlButton label={full ? "Exit full screen (F)" : "Full screen (F)"} onClick={toggleFull}>
            {full ? <Minimize size={18} /> : <Maximize size={18} />}
          </CtlButton>
        </div>
      </div>

      {menu && <SettingsMenu sections={p.settings} onClose={() => setMenu(false)} />}

      {p.error && (
        <div className="absolute inset-0 flex items-center justify-center p-6">
          <div className="card max-w-md p-5 text-center">
            <AlertTriangle size={28} className="mx-auto text-warning" />
            <div className="mt-2 text-sm font-medium text-content">{p.error.title}</div>
            <p className="mt-1 text-xs text-content-muted">{p.error.message}</p>
            <div className="mt-4 flex justify-center gap-2">
              <button className="btn-ghost" onClick={p.onBack}>
                <ArrowLeft size={15} /> Back
              </button>
              {p.error.actions}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function CtlButton({
  label,
  onClick,
  children,
  active,
  disabled,
  wide,
}: {
  label: string;
  onClick: () => void;
  children: ReactNode;
  active?: boolean;
  disabled?: boolean;
  wide?: boolean;
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      title={label}
      aria-label={label}
      className={`flex items-center gap-1.5 rounded-md ${wide ? "px-2.5" : "px-2"} py-1.5 text-white/85 transition-colors hover:bg-white/10 hover:text-white disabled:opacity-30 disabled:hover:bg-transparent ${
        active ? "bg-white/10 text-white" : ""
      }`}
    >
      {children}
    </button>
  );
}

/** Small top-bar button used by the player pages. */
export function TopButton({ label, onClick, children, danger, disabled }: { label: string; onClick: () => void; children: ReactNode; danger?: boolean; disabled?: boolean }) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      title={label}
      className={`flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-sm disabled:opacity-40 ${
        danger ? "bg-critical/90 text-white hover:bg-critical" : "text-white/85 hover:bg-white/10 hover:text-white"
      }`}
    >
      {children}
    </button>
  );
}
