import { useState } from "react";
import { RotateCw } from "lucide-react";
import MediaLocations from "./MediaLocations";
import ServerSettingField from "./ServerSettingField";
import { Spinner } from "../ui";
import { api, useApi } from "../../lib/api";
import { errText } from "../../lib/errors";
import type { ServerSettings as Data } from "../../lib/types";

/**
 * Settings → Server: what used to need environment variables (or
 * couchside.env), editable here. Saved values win over the variables and
 * apply when the server restarts, which the banner does in place.
 */
export default function ServerSettings() {
  const { data, reload } = useApi<Data>("/api/settings/server");
  // Unsaved edits by key: a value, or null to go back to the environment.
  const [draft, setDraft] = useState<Record<string, string | null>>({});
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<{ tone: "good" | "critical"; text: string } | null>(null);
  if (!data) return null;

  const groups = [...new Set(data.settings.map((s) => s.group))];
  const changed = Object.keys(draft).length > 0;
  const edit = (key: string, input: string | null) => {
    setMsg(null);
    const s = data.settings.find((x) => x.key === key)!;
    // An emptied field goes back to the environment, except a secret's: that's just nothing typed yet.
    let v: string | null | undefined = input;
    if (v !== null && v.trim() === "") v = s.kind === "secret" ? undefined : null;
    // No edit left when it matches what's saved.
    const same = v === undefined || (v === null ? !s.saved : s.kind !== "secret" && v === s.value);
    const next = { ...draft };
    if (same) delete next[key];
    else next[key] = v!;
    setDraft(next);
  };
  const save = async () => {
    setBusy(true);
    setMsg(null);
    try {
      await api("/api/settings/server", { method: "PUT", json: draft });
      setDraft({});
      await reload();
      setMsg({ tone: "good", text: "Saved. Restart the server to use the new settings." });
    } catch (e) {
      setMsg({ tone: "critical", text: errText(e) });
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      {data.pending && !changed && <RestartCard canRestart={data.canRestart} failed={data.startFailed} />}
      <p className="px-1 text-xs text-content-muted">
        These used to need environment variables. A value set here wins over the variable
        {data.envFile && (
          <>
            {" "}
            and over <span className="mono">{data.envFile}</span>
          </>
        )}
        ; empty fields use the variable, or the default. The port, the data folder and requiring passwords (
        <span className="mono">COUCHSIDE_AUTH</span>) are only set in the environment.
      </p>
      <section className="card p-4">
        <div className="card-title mb-1">Media locations</div>
        <p className="mb-3 text-xs text-content-muted">
          The drives, folders and network shares your media is on. Libraries and the DVR folder go inside one. These apply at once, without a restart.
        </p>
        <MediaLocations />
      </section>
      {groups.map((g) => (
        <section key={g} className="card p-4">
          <div className="card-title mb-3">{g}</div>
          <div className="flex flex-col gap-5">
            {data.settings
              .filter((s) => s.group === g)
              .map((s) => (
                <ServerSettingField key={s.key} s={s} draft={draft[s.key]} onChange={(v) => edit(s.key, v)} />
              ))}
          </div>
        </section>
      ))}
      {(changed || msg) && (
        <div className="card sticky bottom-4 z-10 flex flex-wrap items-center gap-3 p-3" style={{ boxShadow: "var(--shadow-md)" }}>
          {changed && (
            <>
              <button className="btn-primary" disabled={busy} onClick={() => void save()}>
                {busy && <Spinner size={14} />} Save
              </button>
              <button className="btn-quiet" disabled={busy} onClick={() => (setDraft({}), setMsg(null))}>
                Cancel
              </button>
            </>
          )}
          {msg && <span className={`text-xs ${msg.tone === "good" ? "text-good" : "text-critical"}`}>{msg.text}</span>}
        </div>
      )}
    </>
  );
}

/**
 * Saved settings apply on a restart. The server restarts in its own process;
 * this waits for it to answer again and reloads the page.
 */
function RestartCard({ canRestart, failed }: { canRestart: boolean; failed: string }) {
  const [sure, setSure] = useState(false);
  const [state, setState] = useState<"idle" | "restarting" | "slow">("idle");
  const [err, setErr] = useState<string | null>(null);

  const restart = async () => {
    setErr(null);
    try {
      await api("/api/server/restart", { method: "POST" });
    } catch (e) {
      setErr(errText(e));
      return;
    }
    setState("restarting");
    const start = Date.now();
    let down = false;
    for (;;) {
      await new Promise((r) => setTimeout(r, 1000));
      let up = false;
      try {
        up = (await fetch("/api/status", { credentials: "same-origin" })).ok;
      } catch {
        up = false;
      }
      down = down || !up;
      // Back once it went away, or after a few seconds if the restart was too quick to see.
      if (up && (down || Date.now() - start > 4000)) {
        window.location.reload();
        return;
      }
      if (Date.now() - start > 60000) setState("slow");
    }
  };

  return (
    <section className="card flex flex-wrap items-center gap-3 border-warning/40 p-4">
      <div className="min-w-0 flex-1">
        <div className={`text-sm font-semibold ${failed ? "text-critical" : "text-content"}`}>{failed ? "Saved settings not in use" : "Restart to apply"}</div>
        <p className="text-xs text-content-muted">
          {state === "restarting"
            ? "Restarting… the page reloads when the server is back."
            : state === "slow"
              ? "The server hasn't come back yet. Check its log; a setting it can't use is skipped on the next try."
              : failed && !sure
                ? `The last restart couldn't use the saved settings, so the server is running without them: ${failed}. Fix that below, save and restart.`
                : sure
                    ? "Anything playing stops, and recordings pause for a few seconds (they carry on in the same file)."
                  : canRestart
                    ? "Some saved settings aren't in use yet. They're read when the server starts."
                    : "Some saved settings aren't in use yet. Restart Couchside to use them."}
        </p>
        {err && <p className="mt-1 text-xs text-critical">{err}</p>}
      </div>
      {canRestart && state === "idle" && (
        <div className="flex gap-2">
          {sure && (
            <button className="btn-quiet" onClick={() => setSure(false)}>
              Cancel
            </button>
          )}
          <button className="btn-primary" onClick={() => (sure ? void restart() : setSure(true))}>
            <RotateCw size={14} /> {sure ? "Restart now" : "Restart"}
          </button>
        </div>
      )}
      {state !== "idle" && <Spinner />}
    </section>
  );
}
