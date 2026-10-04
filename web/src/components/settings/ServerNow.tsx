import { Pause, Play, Radio } from "lucide-react";
import Link from "../Link";
import ProfileAvatar from "../ProfileAvatar";
import { ErrorNote, Meter, StatTile } from "../ui";
import { useApi } from "../../lib/api";
import { fmtAgo, fmtBytes, fmtClock } from "../../lib/format";
import type { ConnectedClient, SystemInfo } from "../../lib/types";

const busy = (pct: number) => (pct >= 90 ? "critical" : pct >= 75 ? "warning" : undefined);

/** Live server load, who's connected and what they're watching, and the server's streams. */
export default function ServerNow() {
  const { data, error } = useApi<SystemInfo>("/api/system", { pollMs: 3000 });
  if (!data) return error ? <ErrorNote>Couldn't read the server's load: {error}</ErrorNote> : null;
  const { stats, clients } = data;
  const memPct = stats.memTotal ? (stats.memUsed / stats.memTotal) * 100 : 0;
  const scope = stats.scope === "container" ? "container" : "machine";

  return (
    <>
      {stats.available ? (
        <div className="grid gap-3 sm:grid-cols-3">
          <StatTile
            label="CPU"
            value={`${Math.round(stats.cpuPercent)}%`}
            meter={stats.cpuPercent}
            tone={busy(stats.cpuPercent)}
            sub={`of ${+stats.cores.toFixed(1)} core${stats.cores === 1 ? "" : "s"} · whole ${scope}`}
          />
          <StatTile
            label="Memory"
            value={fmtBytes(stats.memUsed)}
            meter={memPct}
            tone={busy(memPct)}
            sub={`of ${fmtBytes(stats.memTotal)} · Couchside itself ${fmtBytes(stats.server.rss)}`}
          />
          <StatTile
            label="Transcoding"
            value={stats.encoders.count ? `${Math.round(stats.encoders.cpuPercent)}%` : "Idle"}
            meter={stats.encoders.cpuPercent}
            sub={
              stats.encoders.count
                ? `${stats.encoders.count} ffmpeg process${stats.encoders.count === 1 ? "" : "es"} · ${fmtBytes(stats.encoders.rss)}`
                : "No ffmpeg running"
            }
          />
        </div>
      ) : (
        <div className="card px-4 py-3 text-xs text-content-muted">CPU and memory readings are only available when the server runs on Linux.</div>
      )}

      <section className="card p-4">
        <div className="mb-3 flex items-baseline justify-between gap-3">
          <div className="card-title">Connected</div>
          <span className="text-xs text-content-muted">
            {clients.length} {clients.length === 1 ? "person" : "people"} · {clients.filter((c) => c.playing).length} watching
          </span>
        </div>
        <ul className="flex flex-col divide-y divide-border-light">
          {clients.map((c) => (
            <ClientRow key={`${c.profileId}|${c.ip}|${c.device}`} c={c} />
          ))}
        </ul>
        <StreamsLine data={data} />
      </section>
    </>
  );
}

function ClientRow({ c }: { c: ConnectedClient }) {
  const p = c.playing;
  const Icon = !p ? null : p.kind !== "file" ? Radio : p.paused ? Pause : Play;
  return (
    <li className="flex items-start gap-3 py-2.5 first:pt-0 last:pb-0">
      <ProfileAvatar profile={{ name: c.name || "?", color: c.color || "accent" }} size={30} className="mt-0.5 shrink-0" />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-baseline gap-x-2 text-sm">
          <span className="font-medium text-content">{c.name || "Unknown profile"}</span>
          {c.you && <span className="text-xs text-content-muted">(you)</span>}
          <span className="text-xs text-content-muted">
            {c.device} · {c.ip} · since {fmtAgo(Date.now() / 1000 - c.connectedSec).replace(" ago", "")}
          </span>
        </div>
        {p && Icon ? (
          <div className="mt-1">
            <div className="flex items-center gap-1.5 text-xs">
              <Icon size={12} className={p.paused ? "shrink-0 text-content-muted" : "shrink-0 text-accent"} />
              {p.fileId ? (
                <Link to={`/play/${p.fileId}`} className="truncate font-medium text-content-secondary hover:text-accent">
                  {p.title}
                </Link>
              ) : (
                <span className="truncate font-medium text-content-secondary">{p.title}</span>
              )}
              {p.subtitle && <span className="truncate text-content-muted">· {p.subtitle}</span>}
            </div>
            <div className="mt-0.5 flex items-center gap-2 text-xs text-content-muted">
              <span className="shrink-0">{[p.paused ? "Paused" : null, p.mode].filter(Boolean).join(" · ")}</span>
              {p.kind === "file" && p.durationSec > 0 && (
                <>
                  <Meter value={(p.positionSec / p.durationSec) * 100} className="max-w-40" />
                  <span className="shrink-0 tabular-nums">
                    {fmtClock(p.positionSec)} / {fmtClock(p.durationSec)}
                  </span>
                </>
              )}
            </div>
          </div>
        ) : (
          <div className="mt-0.5 text-xs text-content-muted">Browsing</div>
        )}
      </div>
    </li>
  );
}

/** One line on what the server is producing: transcodes, live TV, recordings, encodes. */
function StreamsLine({ data }: { data: SystemInfo }) {
  const transcoding = data.transcodes.filter((t) => t.mode === "transcode").length;
  const remux = data.transcodes.length - transcoding;
  const engine = data.hwaccel === "none" ? "CPU" : data.hwaccel.toUpperCase();
  const parts = [
    `${data.transcodes.length} of ${data.maxStreams} server streams`,
    transcoding ? `${transcoding} transcoding on ${engine}` : null,
    remux ? `${remux} direct stream${remux === 1 ? "" : "s"}` : null,
    data.livetv.liveSessions ? `${data.livetv.liveSessions} live TV channel${data.livetv.liveSessions === 1 ? "" : "s"}` : null,
    data.livetv.recording ? `${data.livetv.recording} recording now` : null,
    data.jobs.running ? `${data.jobs.running} background job${data.jobs.running === 1 ? "" : "s"}${data.jobs.current ? ` (${data.jobs.current})` : ""}` : null,
  ].filter(Boolean);
  return (
    <div className="mt-3 flex flex-wrap items-baseline justify-between gap-2 border-t border-border-light pt-3 text-xs text-content-muted">
      <span>{parts.join(" · ")}</span>
      <Link to="/activity" className="text-content-secondary hover:text-accent">
        Details on Activity
      </Link>
    </div>
  );
}
