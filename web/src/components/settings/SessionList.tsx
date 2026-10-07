import { useState } from "react";
import { Globe, LogOut, Tv } from "lucide-react";
import { ErrorNote, Spinner } from "../ui";
import { api, useApi } from "../../lib/api";
import { fmtAgo } from "../../lib/format";
import type { DeviceSession } from "../../lib/types";
import { errText } from "../../lib/errors";

/** "Chrome on Windows" from a user agent, near enough to recognise a device. */
export function describeAgent(ua: string): string {
  const browser = /Edg\//.test(ua)
    ? "Edge"
    : /Firefox\//.test(ua)
      ? "Firefox"
      : /Chrome\//.test(ua)
        ? "Chrome"
        : /Safari\//.test(ua)
          ? "Safari"
          : "";
  const os = /Windows/.test(ua)
    ? "Windows"
    : /iPhone|iPad/.test(ua)
      ? "iOS"
      : /Android/.test(ua)
        ? "Android"
        : /Mac OS X/.test(ua)
          ? "macOS"
          : /Linux/.test(ua)
            ? "Linux"
            : "";
  if (browser && os) return `${browser} on ${os}`;
  return browser || os || "Unknown device";
}

/** A profile's signed-in devices, each with Sign out. url lists them. */
export default function SessionList({ url }: { url: string }) {
  const { data, error, loading, reload } = useApi<DeviceSession[]>(url);
  const [busy, setBusy] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const end = async (id: string) => {
    setBusy(id);
    setErr(null);
    try {
      await api(`/api/auth/sessions/${encodeURIComponent(id)}`, { method: "DELETE" });
      await reload();
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(null);
    }
  };

  if (loading && !data) return <Spinner />;
  if (error) return <ErrorNote>{error}</ErrorNote>;
  if (!data?.length) return <p className="text-xs text-content-muted">Not signed in anywhere.</p>;
  return (
    <div className="flex flex-col gap-2">
      {err && <ErrorNote>{err}</ErrorNote>}
      <ul className="divide-y divide-border-light rounded-md border border-border-light">
        {data.map((s) => (
          <li key={s.id} className="flex items-center gap-3 px-3 py-2 text-sm">
            {s.client === "tv" ? <Tv size={16} className="shrink-0 text-content-muted" /> : <Globe size={16} className="shrink-0 text-content-muted" />}
            <div className="min-w-0 flex-1">
              <div className="truncate text-content">
                {s.device || (s.client === "tv" ? "TV app" : describeAgent(s.userAgent))}
                {s.current && <span className="tint-good ml-2 badge">This device</span>}
              </div>
              <div className="truncate text-xs text-content-muted">
                {[s.ip, `active ${fmtAgo(s.lastUsedAt)}`, `signed in ${new Date(s.createdAt * 1000).toLocaleDateString()}`].filter(Boolean).join(" · ")}
              </div>
            </div>
            {!s.current && (
              <button className="btn-quiet !text-xs" disabled={busy !== null} onClick={() => void end(s.id)}>
                <LogOut size={13} /> Sign out
              </button>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}
