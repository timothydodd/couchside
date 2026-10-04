import { useRef, useState, type PointerEvent } from "react";
import type { Segment } from "../../lib/types";

const SLIDE = 10; // px a touch must move along the bar before it scrubs

/**
 * Scrubber. [min, max] is the whole bar; buffered and recorded ranges are
 * drawn inside it. When recordedEnd is set (a recording still in progress),
 * the part after it is shown as not-yet-recorded and can't be seeked into.
 * Likewise before availableStart (live TV from before the stream started).
 * Commercial breaks are marked over the track, and marks (where a movie's
 * next part begins) as ticks.
 *
 * On a touch screen a tap does nothing: the finger has to slide along the bar
 * first, so reaching for the buttons below (or swiping up from the bottom
 * edge) doesn't jump the video.
 */
export default function SeekBar({
  min,
  max,
  value,
  bufferedEnd,
  recordedEnd,
  availableStart,
  breaks,
  marks,
  label,
  onSeek,
  onBreakMenu,
}: {
  min: number;
  max: number;
  value: number;
  bufferedEnd: number;
  recordedEnd?: number;
  availableStart?: number;
  breaks?: Segment[];
  marks?: number[];
  label: (t: number) => string;
  onSeek: (t: number) => void;
  /** Right-click (or long-press) on a break: x is the pointer's offset in the bar, in px. */
  onBreakMenu?: (b: Segment, x: number) => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [hover, setHover] = useState<number | null>(null);
  const [drag, setDrag] = useState<number | null>(null);
  const touch = useRef<{ x: number; y: number } | null>(null); // a touch that hasn't started scrubbing
  const span = Math.max(0.001, max - min);
  const limit = recordedEnd ?? max;
  const first = Math.max(min, availableStart ?? min);
  const pct = (t: number) => `${Math.max(0, Math.min(100, ((t - min) / span) * 100))}%`;

  const at = (e: PointerEvent) => {
    const r = ref.current!.getBoundingClientRect();
    const t = min + ((e.clientX - r.left) / r.width) * span;
    return Math.max(first, Math.min(limit, t));
  };

  const shown = drag ?? value;
  return (
    <div
      ref={ref}
      className="group/seek relative h-5 cursor-pointer touch-none select-none pointer-coarse:h-8"
      onPointerMove={(e) => {
        const t = touch.current;
        if (t) {
          const dx = Math.abs(e.clientX - t.x);
          const dy = Math.abs(e.clientY - t.y);
          if (dx < SLIDE && dy < SLIDE) return;
          touch.current = null;
          if (dy > dx) return; // a vertical swipe, not a scrub
          setDrag(at(e));
        }
        if (e.pointerType !== "touch" || drag !== null) setHover(at(e));
        if (drag !== null) setDrag(at(e));
      }}
      onPointerLeave={() => setHover(null)}
      onPointerDown={(e) => {
        if (e.button !== 0) return; // only the main button scrubs; a right-click may open the break menu
        e.currentTarget.setPointerCapture(e.pointerId);
        if (e.pointerType === "touch") touch.current = { x: e.clientX, y: e.clientY };
        else setDrag(at(e));
      }}
      onPointerUp={(e) => {
        if (drag !== null) onSeek(at(e));
        touch.current = null;
        setDrag(null);
        if (e.pointerType === "touch") setHover(null);
      }}
      onPointerCancel={() => {
        touch.current = null;
        setDrag(null);
        setHover(null);
      }}
      onContextMenu={(e) => {
        if (!onBreakMenu || !breaks) return;
        const r = ref.current!.getBoundingClientRect();
        const t = min + ((e.clientX - r.left) / r.width) * span;
        const b = breaks.find((x) => t >= x.start && t < x.end);
        if (!b) return;
        e.preventDefault();
        setDrag(null);
        onBreakMenu(b, e.clientX - r.left);
      }}
      role="slider"
      aria-label="Seek"
      aria-valuemin={min}
      aria-valuemax={max}
      aria-valuenow={Math.round(value)}
    >
      <div className="absolute inset-x-0 top-1/2 h-1 -translate-y-1/2 overflow-hidden rounded-full bg-white/15 transition-[height] group-hover/seek:h-1.5">
        {recordedEnd !== undefined && (
          // not-yet-recorded part of the program
          <div className="absolute inset-y-0 right-0 bg-[repeating-linear-gradient(135deg,rgba(255,255,255,0.06)_0_4px,transparent_4px_8px)]" style={{ left: pct(limit) }} />
        )}
        {recordedEnd !== undefined && <div className="absolute inset-y-0 bg-white/20" style={{ left: pct(first), right: `calc(100% - ${pct(limit)})` }} />}
        <div className="absolute inset-y-0 bg-white/35" style={{ left: pct(first), right: `calc(100% - ${pct(Math.max(first, Math.min(bufferedEnd, limit)))})` }} />
        <div className="absolute inset-y-0 left-0 bg-accent" style={{ width: pct(shown) }} />
        {first > min && (
          // before the stream started: shown as progress through the program, but can't be seeked into
          <div className="absolute inset-y-0 left-0 bg-black/35 bg-[repeating-linear-gradient(135deg,rgba(255,255,255,0.12)_0_4px,transparent_4px_8px)]" style={{ width: pct(first) }} />
        )}
        {breaks?.map((b) => (
          <div key={b.start} className="seek-break" style={{ left: pct(b.start), right: `calc(100% - ${pct(b.end)})` }} />
        ))}
        {marks?.map((m) => (
          <div key={m} className="seek-mark" style={{ left: pct(m) }} />
        ))}
      </div>
      {recordedEnd !== undefined && (
        <div className="absolute top-1/2 h-3 w-0.5 -translate-y-1/2 rounded bg-critical" style={{ left: pct(limit) }} title="Recorded so far" />
      )}
      <div
        className="absolute top-1/2 h-3.5 w-3.5 -translate-x-1/2 -translate-y-1/2 scale-0 rounded-full bg-white shadow transition-transform group-hover/seek:scale-100 pointer-coarse:scale-100"
        style={{ left: pct(shown), transform: drag !== null ? "translate(-50%,-50%) scale(1)" : undefined }}
      />
      {hover !== null && (
        <div className="pointer-events-none absolute -top-8 -translate-x-1/2 rounded bg-black/85 px-2 py-0.5 text-xs tabular-nums text-white" style={{ left: pct(hover) }}>
          {label(hover)}
          {breaks?.some((b) => hover >= b.start && hover < b.end) && " · Commercial"}
        </div>
      )}
    </div>
  );
}
