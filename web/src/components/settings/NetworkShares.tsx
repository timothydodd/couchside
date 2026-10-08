import { useEffect, useState } from "react";
import { CheckCircle2, Plus, Trash2, XCircle } from "lucide-react";
import { Spinner } from "../ui";
import { api, useApi } from "../../lib/api";
import { errText } from "../../lib/errors";
import type { NetworkShares as Data } from "../../lib/types";

interface Row {
  path: string;
  user: string;
  password: string; // typed; empty keeps the saved one
  hasPassword: boolean;
  error?: string; // from the last sign-in; undefined for a new row
}

/**
 * Settings → Server → Network shares (Windows only): user names and
 * passwords for NAS shares the service's account can't read. Saving signs
 * in straight away; the server signs in again each time it starts.
 */
export default function NetworkShares() {
  const { data, reload } = useApi<Data>("/api/settings/shares");
  const [rows, setRows] = useState<Row[]>([]);
  const [dirty, setDirty] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const reset = () => {
    setRows((data?.shares ?? []).map((s) => ({ ...s, password: "" })));
    setDirty(false);
    setErr(null);
  };
  useEffect(reset, [data]); // eslint-disable-line react-hooks/exhaustive-deps
  if (!data?.supported) return null;

  const set = (i: number, p: Partial<Row>) => {
    setErr(null);
    setDirty(true);
    setRows(rows.map((r, j) => (j === i ? { ...r, ...p } : r)));
  };
  const save = async () => {
    setBusy(true);
    setErr(null);
    try {
      await api("/api/settings/shares", {
        method: "PUT",
        json: rows.map((r) => ({ path: r.path, user: r.user, password: r.password || (r.hasPassword ? null : "") })),
      });
      await reload();
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="card p-4">
      <div className="card-title mb-1">Network shares</div>
      <p className="mb-3 text-xs text-content-muted">
        For a NAS that asks for a user name and password. Couchside signs in to these whenever it starts, so you can use them as the media folder and for
        libraries without changing the account the service runs as. Passwords are kept encrypted for this computer and are never shown again.
      </p>
      <div className="flex flex-col gap-3">
        {rows.map((r, i) => (
          <div key={i} className="rounded-md border border-border-light p-3">
            <div className="grid gap-2 sm:grid-cols-[1fr_auto]">
              <input className="field mono w-full" placeholder={"\\\\nas\\media"} value={r.path} onChange={(e) => set(i, { path: e.target.value })} aria-label="Share" />
              <button type="button" className="btn-quiet justify-self-start" onClick={() => (setDirty(true), setRows(rows.filter((_, j) => j !== i)))}>
                <Trash2 size={14} /> Remove
              </button>
            </div>
            <div className="mt-2 grid gap-2 sm:grid-cols-2">
              <input className="field w-full" placeholder="User name (nas\you or you)" autoComplete="off" value={r.user} onChange={(e) => set(i, { user: e.target.value })} aria-label="User name" />
              <input
                className="field w-full"
                type="password"
                autoComplete="new-password"
                placeholder={r.hasPassword ? "Saved; type to replace" : "Password"}
                value={r.password}
                onChange={(e) => set(i, { password: e.target.value })}
                aria-label="Password"
              />
            </div>
            {r.error !== undefined && !dirty && (
              <p className={`mt-2 flex items-center gap-1.5 text-xs ${r.error ? "text-critical" : "text-good"}`}>
                {r.error ? <XCircle size={13} /> : <CheckCircle2 size={13} />} {r.error || "Signed in"}
              </p>
            )}
          </div>
        ))}
      </div>
      <div className="mt-3 flex flex-wrap items-center gap-2">
        <button type="button" className="btn-ghost" onClick={() => (setDirty(true), setRows([...rows, { path: "", user: "", password: "", hasPassword: false }]))}>
          <Plus size={14} /> Add share
        </button>
        {dirty && (
          <>
            <button className="btn-primary" disabled={busy || rows.some((r) => !r.path.trim())} onClick={() => void save()}>
              {busy && <Spinner size={14} />} Save and sign in
            </button>
            <button className="btn-quiet" disabled={busy} onClick={reset}>
              Cancel
            </button>
          </>
        )}
        {err && <span className="text-xs text-critical">{err}</span>}
      </div>
    </section>
  );
}
