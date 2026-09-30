import { useRef, useState, type PointerEvent } from "react";
import type { Segment } from "../../lib/types";

/**
 * Scrubber. [min, max] is the whole bar; buffered and recorded ranges are
 * drawn inside it. When recordedEnd is set (a recording still in progress),
 * the part after it is shown as not-yet-recorded and can't be seeked into.
 * Commercial breaks are marked over the track, and marks (where a movie's
 * next part begins) as ticks.
 */
export default function SeekBar({
  min,
  max,
  value,
  bufferedEnd,
  recordedEnd,
  breaks,
  marks,
  label,
  onSeek,
}: {
  min: number;
  max: number;
  value: number;
  bufferedEnd: number;
  recordedEnd?: number;
  breaks?: Segment[];
  marks?: number[];
  label: (t: number) => string;
  onSeek: (t: number) => void;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [hover, setHover] = useState<number | null>(null);
  const [drag, setDrag] = useState<number | null>(null);
  const span = Math.max(0.001, max - min);
  const limit = recordedEnd ?? max;
  const pct = (t: number) => `${Math.max(0, Math.min(100, ((t - min) / span) * 100))}%`;

  const at = (e: PointerEvent) => {
    const r = ref.current!.getBoundingClientRect();
    const t = min + ((e.clientX - r.left) / r.width) * span;
    return Math.max(min, Math.min(limit, t));
  };

  const shown = drag ?? value;
  return (
    <div
      ref={ref}
      className="group/seek relative h-5 cursor-pointer touch-none select-none"
      onPointerMove={(e) => {
        setHover(at(e));
        if (drag !== null) setDrag(at(e));
      }}
      onPointerLeave={() => setHover(null)}
      onPointerDown={(e) => {
        e.currentTarget.setPointerCapture(e.pointerId);
        setDrag(at(e));
      }}
      onPointerUp={(e) => {
        if (drag !== null) onSeek(at(e));
        setDrag(null);
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
        {recordedEnd !== undefined && <div className="absolute inset-y-0 left-0 bg-white/20" style={{ width: pct(limit) }} />}
        <div className="absolute inset-y-0 left-0 bg-white/35" style={{ width: pct(Math.min(bufferedEnd, limit)) }} />
        <div className="absolute inset-y-0 left-0 bg-accent" style={{ width: pct(shown) }} />
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
        className="absolute top-1/2 h-3.5 w-3.5 -translate-x-1/2 -translate-y-1/2 scale-0 rounded-full bg-white shadow transition-transform group-hover/seek:scale-100"
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
