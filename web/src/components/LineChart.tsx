import { useLayoutEffect, useRef, useState } from "react";
import { fmtTime } from "../lib/format";

export interface ChartSeries<P> {
  label: string;
  /** A token, e.g. "var(--good)". */
  color: string;
  value: (p: P) => number;
  /** Fill under the line: for the total the other series are part of. */
  area?: boolean;
}

const AXIS = 56; // px for the y labels
const PAD_TOP = 6;
const PAD_BOTTOM = 5; // room for the "0" label under the baseline

/**
 * A small time-series chart: lines over [from, to] (unix seconds) and
 * [0, max], broken where readings stopped (gaps over 2.5 × every), with a
 * readout of every series at the pointer.
 */
export default function LineChart<P extends { t: number }>({
  points,
  series,
  from,
  to,
  max,
  every,
  format,
  height = 140,
  label,
}: {
  points: P[];
  series: ChartSeries<P>[];
  from: number;
  to: number;
  max: number;
  every: number;
  format: (v: number) => string;
  height?: number;
  label: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  const [hover, setHover] = useState<P | null>(null);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => setWidth(e.contentRect.width));
    ro.observe(el);
    setWidth(el.clientWidth);
    return () => ro.disconnect();
  }, []);

  const plotW = Math.max(1, width - AXIS);
  const plotH = height - PAD_TOP - PAD_BOTTOM;
  const span = Math.max(1, to - from);
  const top = max > 0 ? max : 1;
  const x = (t: number) => AXIS + ((t - from) / span) * plotW;
  const y = (v: number) => PAD_TOP + plotH - (Math.max(0, Math.min(v, top)) / top) * plotH;

  // Runs of points without a gap between them.
  const runs: P[][] = [];
  for (const p of points) {
    const run = runs[runs.length - 1];
    if (run && p.t - run[run.length - 1].t <= every * 2.5) run.push(p);
    else runs.push([p]);
  }
  const line = (run: P[], s: ChartSeries<P>) => run.map((p, i) => `${i ? "L" : "M"}${x(p.t).toFixed(1)},${y(s.value(p)).toFixed(1)}`).join("");
  const area = (run: P[], s: ChartSeries<P>) => `${line(run, s)}L${x(run[run.length - 1].t).toFixed(1)},${y(0)}L${x(run[0].t).toFixed(1)},${y(0)}Z`;

  const nearest = (clientX: number) => {
    const r = ref.current!.getBoundingClientRect();
    const t = from + ((clientX - r.left - AXIS) / plotW) * span;
    let best: P | null = null;
    for (const p of points) if (!best || Math.abs(p.t - t) < Math.abs(best.t - t)) best = p;
    return best && Math.abs(best.t - t) <= Math.max(every * 2, span / 60) ? best : null;
  };

  const tipLeft = hover ? Math.min(Math.max(x(hover.t), AXIS + 70), width - 70) : 0;
  return (
    <div ref={ref} className="chart" style={{ height: height + 18 }}>
      {width > 0 && (
        <svg
          width={width}
          height={height}
          role="img"
          aria-label={label}
          onPointerMove={(e) => setHover(nearest(e.clientX))}
          onPointerLeave={() => setHover(null)}
        >
          {[0, 0.5, 1].map((f) => (
            <g key={f}>
              <line className="chart-grid" x1={AXIS} x2={width} y1={y(top * f)} y2={y(top * f)} />
              <text className="chart-axis" x={AXIS - 6} y={y(top * f) + 3} textAnchor="end">
                {format(top * f)}
              </text>
            </g>
          ))}
          {series.map((s) =>
            runs.map((run, i) => (
              <g key={`${s.label}-${i}`}>
                {s.area && run.length > 1 && <path d={area(run, s)} fill={s.color} opacity={0.12} />}
                {run.length > 1 ? (
                  <path d={line(run, s)} fill="none" stroke={s.color} strokeWidth={1.5} strokeLinejoin="round" />
                ) : (
                  <circle cx={x(run[0].t)} cy={y(s.value(run[0]))} r={1.5} fill={s.color} />
                )}
              </g>
            )),
          )}
          {hover && (
            <g>
              <line className="chart-grid" x1={x(hover.t)} x2={x(hover.t)} y1={PAD_TOP} y2={y(0)} />
              {series.map((s) => (
                <circle key={s.label} cx={x(hover.t)} cy={y(s.value(hover))} r={3} fill={s.color} />
              ))}
            </g>
          )}
        </svg>
      )}
      <div className="chart-axis flex justify-between" style={{ paddingLeft: AXIS }}>
        <span>{fmtTime(from)}</span>
        <span>{fmtTime(from + span / 2)}</span>
        <span>Now</span>
      </div>
      {hover && (
        <div className="chart-tip" style={{ left: tipLeft, top: 0 }}>
          <div className="mb-0.5 text-content-muted">{new Date(hover.t * 1000).toLocaleTimeString()}</div>
          {series.map((s) => (
            <div key={s.label} className="flex items-center gap-1.5 whitespace-nowrap">
              <span className="chart-swatch" style={{ background: s.color }} />
              <span className="text-content-secondary">{s.label}</span>
              <span className="ml-auto pl-3 tabular-nums text-content">{format(s.value(hover))}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
