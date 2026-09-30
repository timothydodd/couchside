import { useState } from "react";
import { AlertTriangle, CheckCircle2, FolderOpen, RefreshCw, XCircle } from "lucide-react";
import FolderPicker from "../components/FolderPicker";
import Link from "../components/Link";
import ProfileAvatar from "../components/ProfileAvatar";
import { api, useApi } from "../lib/api";
import { fmtAgo, fmtDay, fmtTime } from "../lib/format";
import { BREAK_MODES, SUBTITLE_LANGS } from "../lib/prefs";
import { languageName } from "../lib/tracks";
import type { LiveTvStatus } from "../lib/types";
import { PageHeader, Segmented } from "../components/ui";
import { setTheme, useProfile } from "../stores/profile";
import { useStatus } from "../stores/status";
import { useThemeStore, type ThemePref } from "../stores/theme";

export default function SettingsPage() {
  const status = useStatus((s) => s.status);
  const omdb = status?.providers.includes("omdb");

  return (
    <div>
      <PageHeader title="Settings" />
      <div className="flex max-w-3xl flex-col gap-4 px-6 py-5">
        <ProfileSettings />

        <section className="card p-4">
          <div className="card-title mb-3">Metadata</div>
          <div className="flex items-start gap-3">
            {omdb ? <CheckCircle2 size={18} className="mt-0.5 text-good" /> : <XCircle size={18} className="mt-0.5 text-content-muted" />}
            <div className="text-sm">
              <div className="font-medium text-content">Open Movie Database (OMDb)</div>
              <p className="mt-0.5 text-xs text-content-muted">
                {omdb
                  ? "Connected. Titles, plots, ratings and posters come from OMDb; responses are cached for 30 days."
                  : "Not configured. Get a free key at omdbapi.com and set OMDB_API_KEY on the server (the Helm chart reads it from a Secret)."}
              </p>
            </div>
          </div>
        </section>

        <LiveTvSettings />

        <section className="card p-4">
          <div className="card-title mb-3">Transcoding</div>
          <dl className="grid grid-cols-[160px_1fr] gap-y-1.5 text-sm">
            <dt className="text-content-muted">Encoder</dt>
            <dd>
              {status?.transcode ? (status.transcode.hwaccel === "none" ? "Software (CPU)" : `Hardware: ${status.transcode.hwaccel.toUpperCase()}`) : "–"}
              {status?.transcode && status.transcode.requested !== "none" && status.transcode.hwaccel === "none" && (
                <span className="ml-2 text-xs text-warning">
                  {status.transcode.requested.toUpperCase()} was requested but isn't usable; check the server log
                </span>
              )}
            </dd>
            <dt className="text-content-muted">HDR tone mapping</dt>
            <dd>{status?.transcode ? (status.transcode.tonemap ? "Available" : "Unavailable (HDR will look washed out)") : "–"}</dd>
            <dt className="text-content-muted">Live streams</dt>
            <dd>{status?.transcode ? `${status.transcode.active} of ${status.transcode.maxSessions}` : "–"}</dd>
            <dt className="text-content-muted">Optimized copies</dt>
            <dd>{status?.transcode ? `Up to ${status.transcode.optimizeHeight}p, ${status.transcode.encodeWorkers} at a time` : "–"}</dd>
          </dl>
          <p className="mt-3 text-xs text-content-muted">
            Set <span className="mono">COUCHSIDE_HWACCEL=vaapi</span> (Intel/AMD, needs <span className="mono">/dev/dri</span>), <span className="mono">qsv</span> or{" "}
            <span className="mono">nvenc</span> on the server to use a GPU. Unusable settings fall back to software automatically.
          </p>
        </section>

        <section className="card p-4">
          <div className="card-title mb-3">Server</div>
          <dl className="grid grid-cols-[160px_1fr] gap-y-1.5 text-sm">
            <dt className="text-content-muted">Version</dt>
            <dd className="mono">{status?.version ?? "–"}</dd>
            <dt className="text-content-muted">Media root</dt>
            <dd className="mono">{status?.mediaRoot || "Not restricted"}</dd>
            <dt className="text-content-muted">Automatic rescan</dt>
            <dd>{status?.scanEvery === "0s" ? "Off" : `Every ${status?.scanEvery ?? "–"}`}</dd>
            <dt className="text-content-muted">Background workers</dt>
            <dd>{status?.workers ?? "–"}</dd>
          </dl>
        </section>
      </div>
    </div>
  );
}

/** Preferences of the current profile. Everything else on this page is server-wide. */
function ProfileSettings() {
  const profile = useProfile((s) => s.current);
  const setPrefs = useProfile((s) => s.setPrefs);
  const theme = useThemeStore((s) => s.pref);
  const liveTv = useStatus((s) => s.status?.livetv?.configured);
  if (!profile) return null;
  const p = profile.prefs;
  return (
    <section className="card p-4">
      <div className="mb-4 flex items-center gap-3">
        <ProfileAvatar profile={profile} size={36} />
        <div className="min-w-0 flex-1">
          <div className="card-title">Your settings</div>
          <div className="text-xs text-content-muted">Only for {profile.name}. Other profiles keep their own.</div>
        </div>
        <Link to="/profiles" className="btn-quiet !text-xs">
          Switch or manage profiles
        </Link>
      </div>
      <div className="grid grid-cols-[160px_1fr] items-center gap-x-4 gap-y-3 text-sm">
        <span className="text-content-muted">Theme</span>
        <div>
          <Segmented<ThemePref>
            label="Theme"
            value={theme}
            onChange={setTheme}
            options={[
              { id: "dark", label: "Dark" },
              { id: "light", label: "Light" },
              { id: "system", label: "System" },
            ]}
          />
        </div>
        <span className="text-content-muted">Next episode</span>
        <label className="flex items-center gap-2 text-content-secondary">
          <input type="checkbox" className="accent-brand" checked={p.autoplayNext !== false} onChange={(e) => setPrefs({ autoplayNext: e.target.checked })} />
          Play the next episode automatically
        </label>
        <span className="text-content-muted">Subtitles</span>
        <div>
          <select className="field" value={p.subtitleLang ?? ""} onChange={(e) => setPrefs({ subtitleLang: e.target.value })} aria-label="Subtitles">
            <option value="">Only forced subtitles (foreign dialogue)</option>
            {SUBTITLE_LANGS.map((l) => (
              <option key={l} value={l}>
                Always on in {languageName(l)}, when available
              </option>
            ))}
          </select>
        </div>
        <span className="text-content-muted">Commercials</span>
        <div>
          <Segmented label="Commercials" value={p.commercials ?? "auto"} onChange={(m) => setPrefs({ commercials: m })} options={BREAK_MODES.map((m) => ({ id: m.id, label: m.short }))} />
          <p className="mt-1 text-xs text-content-muted">{BREAK_MODES.find((m) => m.id === (p.commercials ?? "auto"))!.detail}, in recordings that have been checked for commercials.</p>
        </div>
        {liveTv && (
          <>
            <span className="text-content-muted">Live TV quality</span>
            <div>
              <Segmented
                label="Live TV quality"
                value={p.liveHeight ?? 720}
                onChange={(h) => setPrefs({ liveHeight: h })}
                options={[1080, 720, 480].map((h) => ({ id: h, label: `${h}p` }))}
              />
            </div>
          </>
        )}
      </div>
    </section>
  );
}

function LiveTvSettings() {
  const { data: st, reload } = useApi<LiveTvStatus>("/api/livetv/status");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  if (!st) return null;
  const refresh = async () => {
    setBusy(true);
    setErr(null);
    try {
      await api("/api/livetv/refresh", { method: "POST" });
      await reload();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <section className="card p-4">
      <div className="mb-3 flex items-center justify-between">
        <div className="card-title">Live TV &amp; DVR</div>
        {st.configured && (
          <button className="btn-quiet !text-xs" disabled={busy} onClick={() => void refresh()}>
            <RefreshCw size={13} className={busy ? "animate-spin" : ""} /> Refresh channels &amp; guide
          </button>
        )}
      </div>
      {!st.configured ? (
        <p className="text-xs text-content-muted">
          Set <span className="mono">COUCHSIDE_HDHOMERUN</span> to your HDHomeRun's IP address and a writable{" "}
          <span className="mono">COUCHSIDE_RECORDINGS_DIR</span>, then restart.
        </p>
      ) : (
        <dl className="grid grid-cols-[160px_1fr] gap-y-1.5 text-sm">
          <dt className="text-content-muted">Tuner</dt>
          <dd>{st.device ? `${st.device.FriendlyName} (${st.device.DeviceID}), ${st.device.TunerCount} tuners` : <span className="text-critical">{st.error || "Not reachable"}</span>}</dd>
          <dt className="text-content-muted">In use now</dt>
          <dd>
            {st.tunersInUse ?? 0}
            {st.tunerUsers?.length ? <span className="ml-2 text-xs text-content-muted">{st.tunerUsers.join(", ")}</span> : null}
          </dd>
          <dt className="text-content-muted">Guide</dt>
          <dd>
            {st.guideThrough ? `Through ${fmtDay(st.guideThrough)} ${fmtTime(st.guideThrough)}` : "Not loaded yet"}
            {st.guideUpdated ? <span className="ml-2 text-xs text-content-muted">updated {fmtAgo(st.guideUpdated)}</span> : null}
            {st.guideError && <div className="text-xs text-warning">{st.guideError}</div>}
          </dd>
        </dl>
      )}
      {st.configured && <RecordingsFolder onSaved={() => void reload()} />}
      {err && <div className="mt-2 text-xs text-critical">{err}</div>}
    </section>
  );
}

interface FolderOption {
  path: string;
  label: string;
  detail: string;
  writable: boolean;
  problem: string;
}

/** Where the DVR saves recordings. */
function RecordingsFolder({ onSaved }: { onSaved: () => void }) {
  const { data, reload } = useApi<{ recordingsDir: string; default: string; options: FolderOption[]; movable: number; mediaRoot: string }>(
    "/api/dvr/settings",
  );
  const [choice, setChoice] = useState<string | null>(null);
  const [custom, setCustom] = useState("");
  const [browsing, setBrowsing] = useState(false);
  const [move, setMove] = useState(true);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ tone: "good" | "critical"; text: string } | null>(null);
  if (!data) return null;

  const selected = choice ?? data.recordingsDir;
  const isCustom = selected === "__custom";
  const target = isCustom ? custom : selected;
  const changed = target && target !== data.recordingsDir;

  const save = async () => {
    setBusy(true);
    setMsg(null);
    try {
      const r = await api<{ moved: number; failed: string[]; library: string }>("/api/dvr/settings", {
        method: "PUT",
        json: { recordingsDir: target, moveExisting: move && data.movable > 0 },
      });
      const parts = [`New recordings now go to ${target}${r.library ? ` (the ${r.library} library)` : ""}.`];
      if (r.moved) parts.push(`Moved ${r.moved} recording${r.moved === 1 ? "" : "s"}.`);
      if (r.failed?.length) parts.push(`Couldn't move: ${r.failed.join("; ")}`);
      setMsg({ tone: r.failed?.length ? "critical" : "good", text: parts.join(" ") });
      setChoice(null);
      await reload();
      onSaved();
    } catch (e) {
      setMsg({ tone: "critical", text: e instanceof Error ? e.message : String(e) });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="mt-4 border-t border-border-light pt-4">
      <div className="field-label">Save recordings to</div>
      <div className="flex flex-col gap-1.5" role="radiogroup" aria-label="Recordings folder">
        {data.options.map((o) => (
          <label
            key={o.path}
            className={`flex cursor-pointer gap-2.5 rounded-md border px-3 py-2 transition-colors ${selected === o.path ? "border-accent bg-raised" : "border-border-light hover:border-border"}`}
          >
            <input type="radio" name="recdir" className="accent-brand mt-0.5" checked={selected === o.path} onChange={() => setChoice(o.path)} />
            <span className="min-w-0 flex-1">
              <span className="flex items-center gap-2 text-sm text-content">
                {o.label}
                {o.path === data.recordingsDir && <span className="tint-info rounded px-1.5 text-[10px] font-semibold">Current</span>}
              </span>
              <span className="mono block truncate text-content-muted">{o.path}</span>
              <span className="block text-xs text-content-muted">{o.detail}</span>
              {o.problem && (
                <span className="mt-0.5 flex items-center gap-1 text-xs text-warning">
                  <AlertTriangle size={11} /> {o.problem}
                </span>
              )}
            </span>
          </label>
        ))}
        {data.mediaRoot && (
          <label className={`flex cursor-pointer gap-2.5 rounded-md border px-3 py-2 transition-colors ${isCustom ? "border-accent bg-raised" : "border-border-light hover:border-border"}`}>
            <input type="radio" name="recdir" className="accent-brand mt-0.5" checked={isCustom} onChange={() => { setChoice("__custom"); setBrowsing(true); }} />
            <span className="min-w-0 flex-1">
              <span className="block text-sm text-content">Another folder</span>
              {isCustom && (
                <>
                  <div className="mt-1.5 flex gap-2">
                    <input className="field mono flex-1" placeholder={`${data.mediaRoot}/DVR`} value={custom} onChange={(e) => setCustom(e.target.value)} />
                    <button type="button" className="btn-ghost" onClick={() => setBrowsing((b) => !b)}>
                      <FolderOpen size={14} /> Browse
                    </button>
                  </div>
                  {browsing && <FolderPicker path={custom || data.mediaRoot} onPick={setCustom} />}
                  <span className="mt-1 block text-xs text-content-muted">A new folder is created if its parent exists. If it's inside a library, recordings show up in that library.</span>
                </>
              )}
            </span>
          </label>
        )}
      </div>
      {changed && data.movable > 0 && (
        <label className="mt-3 flex items-center gap-2 text-sm text-content-secondary">
          <input type="checkbox" className="accent-brand" checked={move} onChange={(e) => setMove(e.target.checked)} />
          Move my {data.movable} existing recording{data.movable === 1 ? "" : "s"} there too
        </label>
      )}
      {msg && <div className={`tint-${msg.tone} mt-3 rounded-md px-3 py-2 text-xs`}>{msg.text}</div>}
      <button className="btn-primary mt-3" disabled={!changed || busy} onClick={() => void save()}>
        {busy ? "Saving…" : "Save"}
      </button>
    </div>
  );
}
