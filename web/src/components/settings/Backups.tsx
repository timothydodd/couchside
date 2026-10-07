import { useEffect, useState } from "react";
import { DatabaseBackup, Download, Trash2 } from "lucide-react";
import { ErrorNote, Spinner, Loading } from "../ui";
import { api, useApi } from "../../lib/api";
import { fmtAgo, fmtBytes } from "../../lib/format";
import { attempt } from "../../lib/notices";
import { confirmDialog } from "../../lib/ask";

interface Backup {
  name: string;
  kind: "daily" | "manual" | "upgrade";
  size: number;
  at: number;
}

interface Backups {
  backups: Backup[];
  daily: boolean;
  keep: number;
  dir: string;
}

const KIND: Record<Backup["kind"], string> = { daily: "Daily", manual: "Made by hand", upgrade: "Before an upgrade" };

/**
 * Settings → Advanced: copies of the database (with the files that belong
 * with it), made daily and before upgrades. Make one now, download or delete
 * them, and choose how many daily ones to keep.
 */
export default function BackupSettings() {
  const { data, error, reload } = useApi<Backups>("/api/system/backups");
  const [busy, setBusy] = useState(false);
  const [keep, setKeep] = useState("");
  useEffect(() => setKeep(data ? String(data.keep) : ""), [data]);

  const make = attempt("Couldn't make a backup", async () => {
    setBusy(true);
    try {
      await api("/api/system/backups", { method: "POST" });
      await reload();
    } finally {
      setBusy(false);
    }
  });
  const remove = attempt("Couldn't delete the backup", async (b: Backup) => {
    if (!(await confirmDialog({ title: `Delete ${b.name}?`, body: "This backup can't be restored once it's gone.", action: "Delete", danger: true }))) return;
    await api(`/api/system/backups/${encodeURIComponent(b.name)}`, { method: "DELETE" });
    await reload();
  });
  const save = attempt("Couldn't save the backup settings", async (patch: { daily?: boolean; keep?: number }) => {
    if (!data) return;
    await api("/api/settings/backup", { method: "PUT", json: { daily: patch.daily ?? data.daily, keep: patch.keep ?? data.keep } });
    await reload();
  });

  return (
    <section className="card p-4">
      <div className="mb-3 flex items-start justify-between gap-3">
        <div>
          <div className="card-title">Backups</div>
          <div className="text-xs text-content-muted">
            The database, with the key that signs sessions and this server's id. Your media and artwork aren't in it.
          </div>
        </div>
        <button className="btn-primary shrink-0" disabled={busy || !data} onClick={() => void make()}>
          {busy ? <Spinner size={15} /> : <DatabaseBackup size={15} />} Back up now
        </button>
      </div>
      {error && !data && <ErrorNote>Couldn't load the backups: {error}</ErrorNote>}
      {!data && !error && <Loading />}
      {data && (
        <>
          <div className="mb-3 flex flex-wrap items-center gap-x-6 gap-y-2 text-sm">
            <label className="inline-flex items-center gap-2">
              <input type="checkbox" checked={data.daily} onChange={(e) => void save({ daily: e.target.checked })} />
              Back up every day
            </label>
            <label className="inline-flex items-center gap-2">
              Keep the last
              <input
                className="field w-16 !py-1 text-center"
                type="number"
                min={1}
                max={90}
                value={keep}
                onChange={(e) => setKeep(e.target.value)}
                onBlur={() => {
                  const n = Number(keep);
                  if (n >= 1 && n <= 90 && n !== data.keep) void save({ keep: n });
                  else setKeep(String(data.keep));
                }}
                aria-label="Daily backups to keep"
              />
              daily backups
            </label>
          </div>
          {data.backups.length === 0 ? (
            <p className="text-sm text-content-muted">No backups yet. The first daily one is made within the hour.</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="table">
                <thead>
                  <tr>
                    <th>Made</th>
                    <th className="text-right">Size</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  {data.backups.map((b) => (
                    <tr key={b.name}>
                      <td className="whitespace-nowrap">
                        <div className="text-content">{new Date(b.at * 1000).toLocaleString()}</div>
                        <div className="text-xs text-content-muted">
                          {KIND[b.kind]} · {fmtAgo(b.at)}
                        </div>
                      </td>
                      <td className="whitespace-nowrap text-right tabular-nums text-content-secondary">{fmtBytes(b.size)}</td>
                      <td className="text-right">
                        <span className="inline-flex gap-2">
                          <a className="btn-chip" href={`/api/system/backups/${encodeURIComponent(b.name)}`} download={b.name}>
                            <Download size={11} /> Download
                          </a>
                          <button className="btn-chip hover:!text-critical" onClick={() => void remove(b)}>
                            <Trash2 size={11} /> Delete
                          </button>
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <p className="mt-3 text-xs text-content-muted">
            Kept in <span className="mono">{data.dir}</span>. To put one back, stop Couchside and run <span className="mono">couchside restore &lt;file&gt;</span>. A
            backup holds password hashes and the session key, so treat a downloaded one like a password.
          </p>
        </>
      )}
    </section>
  );
}
