import { Film, MonitorPlay } from "lucide-react";
import Link from "./Link";
import { useIsAdmin } from "../stores/auth";
import { useStatus } from "../stores/status";

/** Bottom bar: server reachability, background work, metadata provider. */
export default function StatusBar() {
  const { status, error } = useStatus();
  const jobs = status?.jobs;
  const active = jobs ? jobs.queued + jobs.running : 0;
  // Activity is admin-only; everyone else just sees whether the server is there.
  const admin = useIsAdmin();

  const [dot, label] = error
    ? ["bg-critical", "Server unreachable"]
    : !status
      ? ["bg-warning animate-pulse", "Connecting…"]
      : active
        ? ["bg-info animate-pulse", jobs?.current || `${active} job${active === 1 ? "" : "s"} queued`]
        : ["bg-good", "Idle"];

  return (
    <footer className="flex h-7 shrink-0 items-center gap-4 border-t border-border-light bg-surface px-3 text-[11px] text-content-muted">
      {admin ? (
        <Link to="/activity" className="flex min-w-0 items-center gap-1.5 hover:text-content" title={error ?? undefined}>
          <span className={`h-2 w-2 shrink-0 rounded-full ${dot}`} />
          <span className="truncate text-content-secondary">{label}</span>
          {active > 1 && <span>+{active - 1} more</span>}
        </Link>
      ) : (
        <span className="flex min-w-0 items-center gap-1.5" title={error ?? undefined}>
          <span className={`h-2 w-2 shrink-0 rounded-full ${error ? "bg-critical" : status ? "bg-good" : "bg-warning animate-pulse"}`} />
          <span className="truncate text-content-secondary">{error ? "Server unreachable" : status ? "Connected" : "Connecting…"}</span>
        </span>
      )}
      {admin && !!jobs?.failed && (
        <Link to="/activity" className="text-critical hover:underline">
          {jobs.failed} failed
        </Link>
      )}
      {!!status?.livetv?.recording && (
        <Link to="/livetv/recordings" className="flex items-center gap-1 text-critical hover:underline">
          <span className="h-2 w-2 animate-pulse rounded-full bg-critical" />
          {status.livetv.recording} recording
        </Link>
      )}
      {admin && !!status?.transcode?.active && (
        <Link to="/activity" className="flex items-center gap-1 text-info hover:underline">
          <MonitorPlay size={12} />
          {status.transcode?.active} transcoding
        </Link>
      )}
      <span className="ml-auto flex items-center gap-1">
        <Film size={12} />
        {status?.providers.length ? status.providers.map((p) => p.toUpperCase()).join(" + ") : "No metadata provider"}
      </span>
      {status && <span className="mono">{/^\d/.test(status.version) ? `v${status.version}` : status.version}</span>}
    </footer>
  );
}
