import { useState } from "react";
import LineChart, { type ChartSeries } from "../LineChart";
import { Segmented } from "../ui";
import { useApi } from "../../lib/api";
import { fmtBytes } from "../../lib/format";
import type { HistoryPoint, HistoryWindow } from "../../lib/types";

const RANGES = [
  { id: "15m", label: "15 min", secs: 15 * 60 },
  { id: "1h", label: "1 hour", secs: 3600 },
  { id: "6h", label: "6 hours", secs: 6 * 3600 },
  { id: "24h", label: "24 hours", secs: 24 * 3600 },
] as const;
type Range = (typeof RANGES)[number]["id"];

// Whole machine in grey, Couchside in the logo's blue, its encoders in sky.
const TOTAL = "var(--text-muted)";
const SERVER = "var(--good)";
const ENCODE = "var(--info)";

const pct = (v: number) => `${Math.round(v)}%`;

/** A round top for a chart showing up to peak (1, 2 or 5 × 10ⁿ), never above cap. */
const niceMax = (peak: number, cap: number) => {
  if (!(peak > 0)) return cap;
  const e = 10 ** Math.floor(Math.log10(peak));
  return Math.min(cap, [1, 2, 5, 10].find((k) => k * e >= peak)! * e);
};
const count = (v: number) => String(Math.round(v));

/** CPU, memory and streams over time, kept by the server since it started. */
export default function SystemHistory() {
  const [range, setRange] = useState<Range>("15m");
  const secs = RANGES.find((r) => r.id === range)!.secs;
  const { data } = useApi<HistoryWindow>(`/api/system/history?range=${range}`, { pollMs: secs <= 3600 ? 5000 : 60000 });
  const to = Date.now() / 1000;
  const from = to - secs;
  const scope = data?.scope === "container" ? "Container" : "Machine";
  const pts = data?.points ?? [];
  const last = pts[pts.length - 1];

  const cpu: ChartSeries<HistoryPoint>[] = [
    { label: scope, color: TOTAL, value: (p) => p.cpu, area: true },
    { label: "Couchside", color: SERVER, value: (p) => p.serverCpu },
    { label: "ffmpeg & comskip", color: ENCODE, value: (p) => p.encCpu },
  ];
  const mem: ChartSeries<HistoryPoint>[] = [
    { label: scope, color: TOTAL, value: (p) => p.mem, area: true },
    { label: "Couchside", color: SERVER, value: (p) => p.serverMem },
    { label: "ffmpeg & comskip", color: ENCODE, value: (p) => p.encMem },
  ];
  const work: ChartSeries<HistoryPoint>[] = [
    { label: "Streams", color: SERVER, value: (p) => p.streams },
    { label: "Encoder processes", color: ENCODE, value: (p) => p.encoders },
  ];
  const workMax = Math.max(4, ...pts.map((p) => Math.max(p.streams, p.encoders)));
  // Scale to what was used, so a quiet server isn't a flat line along the bottom.
  const cpuMax = niceMax(Math.max(5, ...pts.map((p) => p.cpu)) * 1.1, 100);
  const memMax = niceMax(Math.max(...pts.map((p) => p.mem)) * 1.1, data?.memTotal || 1);

  return (
    <section className="card p-4">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div>
          <div className="card-title">Over time</div>
          <div className="text-xs text-content-muted">
            {range === "6h" || range === "24h" ? "1-minute averages" : "Every 5 seconds"} · kept in memory since the server started
          </div>
        </div>
        <Segmented label="Time range" value={range} onChange={setRange} options={RANGES.map((r) => ({ id: r.id, label: r.label }))} />
      </div>
      {!data ? null : pts.length < 2 ? (
        <p className="py-6 text-center text-xs text-content-muted">
          Collecting readings… the first points show up within a few seconds. (CPU and memory are only measured on Linux.)
        </p>
      ) : (
        <div className="flex flex-col gap-6">
          <ChartBlock title="CPU" sub={`% of ${+data.cores.toFixed(1)} core${data.cores === 1 ? "" : "s"}`} series={cpu} last={last} format={pct}>
            <LineChart label="CPU over time" points={pts} series={cpu} from={from} to={to} max={cpuMax} every={data.every} format={pct} />
          </ChartBlock>
          <ChartBlock title="Memory" sub={`of ${fmtBytes(data.memTotal)}`} series={mem} last={last} format={fmtBytes}>
            <LineChart label="Memory over time" points={pts} series={mem} from={from} to={to} max={memMax} every={data.every} format={fmtBytes} />
          </ChartBlock>
          <ChartBlock title="Transcoding" sub="streams the server is producing, and ffmpeg/comskip running" series={work} last={last} format={count}>
            <LineChart label="Streams over time" points={pts} series={work} from={from} to={to} max={workMax} every={data.every} format={count} height={90} />
          </ChartBlock>
        </div>
      )}
    </section>
  );
}

function ChartBlock({
  title,
  sub,
  series,
  last,
  format,
  children,
}: {
  title: string;
  sub: string;
  series: ChartSeries<HistoryPoint>[];
  last: HistoryPoint;
  format: (v: number) => string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <div className="mb-2 flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <div className="text-sm">
          <span className="font-medium text-content">{title}</span> <span className="text-xs text-content-muted">{sub}</span>
        </div>
        <div className="flex flex-wrap gap-x-3 gap-y-1 text-xs">
          {series.map((s) => (
            <span key={s.label} className="chart-key">
              <span className="chart-swatch" style={{ background: s.color }} />
              <span className="text-content-muted">{s.label}</span>
              <span className="tabular-nums text-content-secondary">{format(s.value(last))}</span>
            </span>
          ))}
        </div>
      </div>
      {children}
    </div>
  );
}
