import { useCallback, useEffect, useRef, useState, type ReactNode, type RefObject, type VideoHTMLAttributes } from "react";
import {
  AlertTriangle, ArrowLeft, ChevronsRight, ListVideo, Maximize, Minimize, Pause, Play, Radio, RotateCcw, RotateCw, Settings2, SkipBack, SkipForward, Volume1, Volume2, VolumeX,
} from "lucide-react";
import SeekBar from "./SeekBar";
import SettingsMenu, { type SettingSection } from "./SettingsMenu";
import QueueMenu from "./QueueMenu";
import type { QueueEntry } from "../../stores/queue";
import { useMediaState } from "./useMediaState";
import { useBreakSkip } from "./useBreakSkip";
import { useIntroSkip } from "./useIntroSkip";
import { fmtClock, fmtTime } from "../../lib/format";
import type { PreviewFrame } from "../../lib/trickplay";
import type { BreakMode, MarkedSegment, Segment } from "../../lib/types";

/**
 * How the timeline behaves:
 * - vod: a file with a fixed duration. With parts, the file is one part of a
 *   movie and the bar spans the whole movie (see PartsTimeline).
 * - live: a growing live stream you can rewind within; "Live" jumps to the edge.
 *   The bar spans the program being watched in clock time (9:00–10:00 at
 *   9:30 sits in the middle), or the half-hour slot when the guide doesn't
 *   cover it: before the stream started and not-yet-aired are hatched.
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
  /** The file's intro and end credits (vod only), and how to treat the intro. */
  segments?: MarkedSegment[];
  introMode?: BreakMode;
  /** What the button offered during the credits does (the next episode, or leave); omitted, no button. */
  onCredits?: { label: string; go: () => void };
  /** A thumbnail of this file at time t (seconds), for the seek bar (vod only). */
  preview?: (t: number) => PreviewFrame | null;
  breakMode?: BreakMode;
  /** Offered on a right-click on a break in the seek bar; omitted, there's no menu. */
  onNotCommercial?: (b: Segment) => void;
  onBack: () => void;
  /** The play queue (vod): previous/next buttons and the queue overlay. Omitted, there's none. */
  queue?: {
    entries: QueueEntry[];
    current: number; // file id
    onPlay: (fileId: number) => void;
    onRemove: (fileId: number) => void;
  };
  onKey?: (e: KeyboardEvent) => boolean; // return true when handled
  loading?: string | null;
  notice?: string | null;
  /** A button in the notice, e.g. Undo. */
  noticeAction?: { label: string; onClick: () => void } | null;
  error?: { title: string; message: string; actions?: ReactNode } | null;
  videoProps?: VideoHTMLAttributes<HTMLVideoElement>;
  children?: ReactNode; // e.g. <track> elements
}

const LIVE_SLACK = 12; // seconds behind the edge that still counts as "live"

/** The half-hour slot (9:00–9:30, 9:30–10:00) holding clock time t: a stand-in for a missing guide entry. */
const halfHour = (t: number) => {
  const startAt = Math.floor(t / 1800) * 1800;
  return { startAt, endAt: startAt + 1800 };
};

/** The player shell shared by movies/episodes, live TV and recordings. */
export default function PlayerFrame(p: PlayerFrameProps) {
  const { videoRef, timeline } = p;
  const root = useRef<HTMLDivElement>(null);
  const st = useMediaState(videoRef);
  const brk = useBreakSkip(videoRef, st.time, timeline.kind === "vod" ? p.breaks : undefined, p.breakMode ?? "off");
  const seg = useIntroSkip(videoRef, st.time, timeline.kind === "vod" ? p.segments : undefined, p.introMode ?? "button");
  const [chrome, setChrome] = useState(true);
  const [menu, setMenu] = useState(false);
  const [queueOpen, setQueueOpen] = useState(false);
  const resumeAfterQueue = useRef(false);
  const [breakMenu, setBreakMenu] = useState<{ b: Segment; x: number } | null>(null);
  const [full, setFull] = useState(false);
  const [volOpen, setVolOpen] = useState(false);
  const hideTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const tap = useRef<{ touch: boolean; shown: boolean }>({ touch: false, shown: true });

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
  // iPhone Safari can't make an element full screen, only the video itself
  // (its own player, without these controls).
  const toggleFull = useCallback(() => {
    if (document.fullscreenElement) void document.exitFullscreen();
    else if (root.current?.requestFullscreen) void root.current.requestFullscreen().catch(() => {});
    else (videoRef.current as (HTMLVideoElement & { webkitEnterFullscreen?: () => void }) | null)?.webkitEnterFullscreen?.();
  }, [videoRef]);

  useEffect(() => {
    const onFs = () => setFull(!!document.fullscreenElement);
    document.addEventListener("fullscreenchange", onFs);
    // iPhone's own full-screen player doesn't fire fullscreenchange.
    const el = videoRef.current;
    const begin = () => setFull(true);
    const end = () => setFull(false);
    el?.addEventListener("webkitbeginfullscreen", begin);
    el?.addEventListener("webkitendfullscreen", end);
    return () => {
      document.removeEventListener("fullscreenchange", onFs);
      el?.removeEventListener("webkitbeginfullscreen", begin);
      el?.removeEventListener("webkitendfullscreen", end);
    };
  }, [videoRef]);

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
  // The handler reads this render's state, so it's kept in a ref and the
  // listener is added once, not on every render (about four a second).
  const keyHandler = useRef<(e: KeyboardEvent) => void>(() => {});
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => keyHandler.current(e);
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  // --- play queue --------------------------------------------------------------------
  const q = p.queue;
  const qAt = q ? q.entries.findIndex((e) => e.fileId === q.current) : -1;
  const qPrev = q && qAt > 0 ? q.entries[qAt - 1].fileId : null;
  const qNext = q && qAt >= 0 && qAt + 1 < q.entries.length ? q.entries[qAt + 1].fileId : null;
  // Previous: inside the first five seconds it's the item before; after
  // that it starts this one over, like a CD player.
  const atStart = st.time <= 5;
  const canPrevious = !!q && (!atStart || qPrev !== null);
  const previous = useCallback(() => {
    if (!q) return;
    if (st.time > 5 || qPrev === null) seekTo(0);
    else q.onPlay(qPrev);
  }, [q, qPrev, st.time, seekTo]);
  const next = useCallback(() => {
    if (q && qNext !== null) q.onPlay(qNext);
  }, [q, qNext]);
  const openQueue = useCallback(() => {
    const el = v();
    resumeAfterQueue.current = !!el && !el.paused;
    el?.pause();
    setMenu(false);
    setQueueOpen(true);
  }, []);
  const closeQueue = useCallback(() => {
    setQueueOpen(false);
    if (resumeAfterQueue.current) void v()?.play().catch(() => {});
  }, []);

  {
    keyHandler.current = (e: KeyboardEvent) => {
      if ((e.target as HTMLElement)?.closest("input, select, textarea")) return;
      // The page's own shortcuts first: they may use a modifier (Ctrl+Up
      // changes channel).
      if (p.onKey?.(e)) return poke();
      // Alt+Left is the browser's Back, Ctrl+F its Find: leave those alone.
      if (e.ctrlKey || e.metaKey || e.altKey) return;
      poke();
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
        case "n":
          if (q) next();
          break;
        case "p":
          if (q && canPrevious) previous();
          break;
        case "q":
          if (q && q.entries.length > 1) (queueOpen ? closeQueue : openQueue)();
          break;
        case "s":
          if (brk.current) brk.skip();
          else seg.intro?.skip();
          break;
        case "Escape":
          if (menu) setMenu(false);
          else if (!document.fullscreenElement) p.onBack();
          break;
      }
    };
  }

  // --- timeline math ------------------------------------------------------------------
  let min = 0;
  let max = isFinite(st.duration) ? st.duration : st.seekEnd;
  let recordedEnd: number | undefined;
  let label = (t: number) => fmtClock(t);
  let availableStart: number | undefined;
  // Live with a guide: map video time to the clock (the edge is now) and span the
  // program the playhead is in.
  const clockAt = (t: number) => Date.now() / 1000 - (st.seekEnd - t);
  const program = timeline.kind === "live" ? (timeline.programs?.find((g) => clockAt(st.time) >= g.startAt && clockAt(st.time) < g.endAt) ?? halfHour(clockAt(st.time))) : undefined;
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
          : "";

  const VolIcon = st.muted || st.volume === 0 ? VolumeX : st.volume < 0.5 ? Volume1 : Volume2;
  const show = chrome || menu || !!p.error;

  return (
    <div
      ref={root}
      data-theme="dark"
      className="fixed inset-0 z-50 select-none bg-[var(--player-bg)] text-player-fg"
      style={{ cursor: show ? "auto" : "none" }}
      onPointerMove={poke}
      onPointerDown={poke}
    >
      <video
        ref={videoRef}
        className="h-full w-full"
        autoPlay
        playsInline
        onPointerDown={(e) => (tap.current = { touch: e.pointerType === "touch", shown: show })}
        onClick={() => {
          if (menu) setMenu(false);
          else if (!tap.current.touch) toggle();
          // On a touch screen a tap shows or hides the controls; it doesn't pause.
          else if (tap.current.shown && !st.paused) {
            clearTimeout(hideTimer.current);
            setChrome(false);
          }
        }}
        onDoubleClick={toggleFull}
        {...p.videoProps}
      >
        {p.children}
      </video>

      {/* center: loading / paused */}
      {(p.loading || (st.waiting && !st.paused)) && !p.error && (
        <div className="pointer-events-none absolute inset-0 flex flex-col items-center justify-center gap-3">
          <div className="h-10 w-10 animate-spin rounded-full border-2 border-player-fg/20 border-t-white/90" />
          {p.loading && <div className="text-sm text-player-fg/80">{p.loading}</div>}
        </div>
      )}
      {st.paused && !p.loading && !p.error && st.width > 0 && (
        <button
          className="absolute left-1/2 top-1/2 flex h-16 w-16 -translate-x-1/2 -translate-y-1/2 items-center justify-center rounded-full bg-player-bg/55 text-player-fg backdrop-blur transition-transform hover:scale-105"
          onClick={toggle}
          aria-label="Play"
        >
          <Play size={28} className="translate-x-0.5 fill-current" />
        </button>
      )}

      {/* top bar */}
      <div
        className={`absolute inset-x-0 top-0 flex items-start gap-3 bg-gradient-to-b from-player-bg/85 via-player-bg/45 to-transparent player-top pb-14 transition-opacity duration-300 ${
          show ? "opacity-100" : "pointer-events-none opacity-0"
        }`}
      >
        <button onClick={p.onBack} className="mt-0.5 rounded-md p-1.5 text-player-fg/85 hover:bg-player-fg/10 hover:text-player-fg pointer-coarse:p-2.5" aria-label="Back">
          <ArrowLeft size={19} />
        </button>
        {p.logo && <img src={p.logo} alt="" className="channel-logo mt-0.5 max-h-9 max-w-16" />}
        <div className="min-w-0 flex-1">
          {p.badge && <div className="mb-0.5 flex items-center gap-2 text-xs text-player-fg/70">{p.badge}</div>}
          <div className="truncate text-lg font-semibold leading-tight">{p.title}</div>
          {p.subtitle && <div className="truncate text-xs text-player-fg/65">{p.subtitle}</div>}
        </div>
        <div className="flex items-center gap-1">{p.topActions}</div>
      </div>

      {p.notice && !p.error && (
        <div className="tint-info pointer-events-none absolute left-1/2 top-20 max-w-md -translate-x-1/2 rounded-md px-3 py-2 text-center text-xs backdrop-blur">
          {p.notice}
          {p.noticeAction && (
            <button
              className="pointer-events-auto ml-2 font-semibold underline"
              onClick={(e) => {
                e.stopPropagation();
                p.noticeAction!.onClick();
              }}
            >
              {p.noticeAction.label}
            </button>
          )}
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

      {!brk.current && !brk.skipped && !p.error && (seg.intro || (seg.inCredits && p.onCredits)) && (
        <div className="absolute bottom-28 right-5 z-20" onClick={(e) => e.stopPropagation()}>
          {seg.intro ? (
            <button className="player-pill" onClick={seg.intro.skip} title="Skip intro (S)">
              Skip intro <ChevronsRight size={16} />
            </button>
          ) : (
            <button className="player-pill" onClick={p.onCredits!.go}>
              {p.onCredits!.label} <ChevronsRight size={16} />
            </button>
          )}
        </div>
      )}

      {/* bottom controls */}
      <div
        className={`absolute inset-x-0 bottom-0 bg-gradient-to-t from-player-bg/90 via-player-bg/55 to-transparent player-controls pt-16 transition-opacity duration-300 ${
          show ? "opacity-100" : "pointer-events-none opacity-0"
        }`}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="relative">
        {breakMenu && (
          <BreakMenu
            x={breakMenu.x}
            onPick={() => {
              p.onNotCommercial?.({ start: breakMenu.b.start - off, end: breakMenu.b.end - off });
              setBreakMenu(null);
            }}
            onClose={() => setBreakMenu(null)}
          />
        )}
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
          preview={p.preview ? (t) => p.preview!(t - off) : undefined}
          onSeek={(t) => {
            brk.allow(t - off);
            seekBar(t);
          }}
          onBreakMenu={p.onNotCommercial ? (b, x) => setBreakMenu({ b, x }) : undefined}
        />
        </div>
        <div className="mt-1 flex items-center gap-1 pointer-coarse:mt-3">
          <CtlButton label={st.paused ? "Play (Space)" : "Pause (Space)"} onClick={toggle}>
            {st.paused ? <Play size={20} className="fill-current" /> : <Pause size={20} className="fill-current" />}
          </CtlButton>
          {q && (
            <CtlButton label={!atStart ? "Start over (P)" : "Previous (P)"} onClick={previous} disabled={!canPrevious}>
              <SkipBack size={18} />
            </CtlButton>
          )}
          <CtlButton label="Back 10 seconds (←)" onClick={() => skip(-10)}>
            <RotateCcw size={18} />
          </CtlButton>
          <CtlButton label="Forward 30 seconds (L)" onClick={() => skip(30)} disabled={atLive}>
            <RotateCw size={18} />
          </CtlButton>
          {q && (
            <CtlButton label="Next (N)" onClick={next} disabled={qNext === null}>
              <SkipForward size={18} />
            </CtlButton>
          )}
          {/* phones have volume buttons, and iOS ignores the page's volume */}
          <div className="relative flex items-center max-md:hidden" onMouseEnter={() => setVolOpen(true)} onMouseLeave={() => setVolOpen(false)}>
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
          <span className="ml-2 min-w-0 truncate text-xs tabular-nums text-player-fg/80">{timeText}</span>
          <div className="flex-1" />
          {timeline.kind === "recording" && (
            <CtlButton label="Start over" onClick={() => seekTo(0)} wide>
              <SkipBack size={15} /> <span className="hidden text-xs sm:inline">Start over</span>
            </CtlButton>
          )}
          {isLive && (
            <button
              onClick={goLive}
              className={`flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs font-bold tracking-wide ${
                atLive ? "bg-critical text-on-accent" : "bg-player-fg/10 text-player-fg/80 hover:bg-player-fg/20 hover:text-player-fg"
              }`}
              title={atLive ? "You're watching live" : "Jump to live"}
            >
              <Radio size={13} /> {atLive ? "LIVE" : "GO LIVE"}
            </button>
          )}
          {q && q.entries.length > 1 && (
            <CtlButton label="Queue (Q)" onClick={() => (queueOpen ? closeQueue() : openQueue())} active={queueOpen}>
              <ListVideo size={19} />
            </CtlButton>
          )}
          <CtlButton label="Settings" onClick={() => { setQueueOpen(false); setMenu((m) => !m); }} active={menu}>
            <Settings2 size={19} />
          </CtlButton>
          <CtlButton label={full ? "Exit full screen (F)" : "Full screen (F)"} onClick={toggleFull}>
            {full ? <Minimize size={18} /> : <Maximize size={18} />}
          </CtlButton>
        </div>
      </div>

      {menu && <SettingsMenu sections={p.settings} onClose={() => setMenu(false)} />}
      {queueOpen && q && <QueueMenu entries={q.entries} current={q.current} onPlay={(id) => { setQueueOpen(false); q.onPlay(id); }} onRemove={q.onRemove} onClose={closeQueue} />}

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
      className={`player-btn ${wide ? "px-2.5" : "px-2"} ${
        active ? "bg-player-fg/10 text-player-fg" : ""
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
      aria-label={label}
      className={`flex shrink-0 items-center gap-1.5 rounded-md px-2.5 py-1.5 text-sm disabled:opacity-40 ${
        danger ? "bg-critical/90 text-on-accent hover:bg-critical" : "text-player-fg/85 hover:bg-player-fg/10 hover:text-player-fg"
      }`}
    >
      {children}
    </button>
  );
}

/** The menu a right-click on a commercial break opens, just above the seek bar. */
function BreakMenu({ x, onPick, onClose }: { x: number; onPick: () => void; onClose: () => void }) {
  const ref = useRef<HTMLDivElement>(null);
  const close = useRef(onClose); // a new function each render: don't re-subscribe for it
  close.current = onClose;
  useEffect(() => {
    const away = (e: PointerEvent) => !ref.current?.contains(e.target as Node) && close.current();
    const esc = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.stopPropagation(); // close the menu, not the player
      close.current();
    };
    window.addEventListener("pointerdown", away, true);
    window.addEventListener("keydown", esc, true);
    return () => {
      window.removeEventListener("pointerdown", away, true);
      window.removeEventListener("keydown", esc, true);
    };
  }, []);
  return (
    <div ref={ref} className="absolute bottom-6 z-30 -translate-x-1/2" style={{ left: x }} role="menu">
      <button className="player-pill !py-1.5 !text-xs" role="menuitem" autoFocus onClick={onPick}>
        Not a commercial
      </button>
    </div>
  );
}
