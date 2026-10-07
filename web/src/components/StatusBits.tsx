import Link from "./Link";
import { useStatus } from "../stores/status";

/** What the job queue is doing, worded the same everywhere it's shown. */
export function useJobsSummary() {
  const jobs = useStatus((s) => s.status?.jobs);
  const active = jobs ? jobs.queued + jobs.running : 0;
  const failed = jobs?.failed ?? 0;
  const label = !jobs ? "" : active ? jobs.current || `${active} job${active === 1 ? "" : "s"} queued` : failed ? `${failed} failed` : "Idle";
  return { jobs, active, failed, label };
}

/** "REC" with a pulsing dot while something is recording; a link when `to` is given. */
export function RecBadge({ to, className = "" }: { to?: string; className?: string }) {
  const recording = useStatus((s) => s.status?.livetv?.recording ?? 0);
  if (!recording) return null;
  const title = `${recording} recording now`;
  const body = (
    <>
      <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-critical" />
      REC
    </>
  );
  return to ? (
    <Link to={to} className={`badge tint-critical ${className}`} title={title}>
      {body}
    </Link>
  ) : (
    <span className={`badge tint-critical ${className}`} title={title}>
      {body}
    </span>
  );
}

/** The Activity count: failed jobs first (they need someone), else what's running or queued. */
export function JobsBadge() {
  const { jobs, active, failed } = useJobsSummary();
  if (!jobs) return null;
  if (failed)
    return (
      <span className="badge tint-critical tabular-nums" title={`${failed} failed`}>
        {failed} failed
      </span>
    );
  if (active)
    return (
      <span className="badge tint-info tabular-nums" title={`${jobs.running} running, ${jobs.queued} queued`}>
        {active}
      </span>
    );
  return null;
}
