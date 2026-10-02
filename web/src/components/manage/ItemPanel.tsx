import { useEffect, useState } from "react";
import { Cpu, ExternalLink, Trash2, X } from "lucide-react";
import Link from "../Link";
import { PosterArt } from "../PosterCard";
import { ErrorNote } from "../ui";
import ArtworkEditor from "./ArtworkEditor";
import FileList from "./FileList";
import MetadataSearch from "./MetadataSearch";
import { api, useApi } from "../../lib/api";
import { fmtBytes, relPath } from "../../lib/format";
import type { DeleteResult, Library, ManageFile, ManageRow } from "../../lib/types";
import { useStatus } from "../../stores/status";

/** Everything you can do to one movie or show, in a drawer over the Manage table. */
export default function ItemPanel({ row, library, onClose, onChanged }: { row: ManageRow; library: Library; onClose: () => void; onChanged: () => void }) {
  const { data: files, reload: reloadFiles } = useApi<ManageFile[]>(`/api/items/${row.id}/files`);
  const [note, setNote] = useState<string | null>(null);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  useEffect(() => setNote(null), [row.id]);

  const changed = () => {
    void reloadFiles();
    onChanged();
  };
  const deleted = (r: DeleteResult) => {
    const parts = [`Deleted ${r.deleted} file${r.deleted === 1 ? "" : "s"} (${fmtBytes(r.bytes)}).`];
    if (r.keptFolders.length) parts.push(`Left ${r.keptFolders.map((f) => relPath(f, library.path)).join(", ")}: it still holds other files (artwork, .nfo).`);
    setNote(parts.join(" "));
    if (r.itemsRemoved.includes(row.id)) {
      onChanged();
      onClose();
      return;
    }
    changed();
  };

  return (
    <>
      <div className="fixed inset-0 z-30 bg-black/30" onClick={onClose} />
      <aside className="side-panel" role="dialog" aria-label={`Manage ${row.title}`}>
        <div className="flex items-start gap-3 px-4 py-4">
          <div className="poster relative w-16 shrink-0">
            <PosterArt item={row} bare />
          </div>
          <div className="min-w-0 flex-1">
            <div className="text-base font-semibold leading-tight text-content">{row.title}</div>
            <div className="mt-0.5 text-xs text-content-muted">
              {[row.year, row.kind === "series" ? `${row.episodeCount} episodes` : null, `${row.fileCount} file${row.fileCount === 1 ? "" : "s"}`, fmtBytes(row.size)]
                .filter(Boolean)
                .join(" · ")}
            </div>
            <Link to={`/item/${row.id}`} className="mt-1.5 inline-flex items-center gap-1 text-xs text-content-secondary hover:text-accent">
              <ExternalLink size={12} /> Open page
            </Link>
          </div>
          <button className="btn-quiet" onClick={onClose} aria-label="Close">
            <X size={16} />
          </button>
        </div>
        <div className="flex-1 overflow-y-auto">
          {note && (
            <div className="px-4 pb-3">
              <div className="tint-good rounded-md px-3 py-2 text-xs">{note}</div>
            </div>
          )}
          <MetadataSearch row={row} onMatched={onChanged} />
          <ArtworkEditor row={row} onChanged={onChanged} />
          <FileList kind={row.kind} files={files} libraryPath={library.path} onDeleted={deleted} onChanged={changed} />
          <OptimizeItem row={row} />
          <DeleteItem row={row} onDeleted={deleted} />
        </div>
      </aside>
    </>
  );
}

/** Queue background encodes that make browser-friendly copies of this title. */
function OptimizeItem({ row }: { row: ManageRow }) {
  const [msg, setMsg] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => setMsg(null), [row.id]);
  const series = row.kind === "series";
  const run = async () => {
    setBusy(true);
    try {
      const r = await api<{ queued: number }>(`/api/items/${row.id}/optimize`, { method: "POST" });
      setMsg(
        r.queued
          ? `Queued ${r.queued} encode${r.queued === 1 ? "" : "s"}; progress is on the Activity page.`
          : series
            ? "Every episode already plays in the browser or has an optimized copy."
            : "This already plays in the browser or has an optimized copy.",
      );
      void useStatus.getState().refresh();
    } catch (e) {
      setMsg(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <section className="panel-section">
      <div className="card-title mb-2">Optimize</div>
      <p className="text-xs text-content-muted">
        Encodes a browser-friendly H.264 copy of {series ? "each episode" : "this movie"} that can't play directly, so it plays without live transcoding.
        Copies go in the cache volume (roughly 1&ndash;4 GB per movie at 1080p).
      </p>
      <div className="mt-3 flex flex-wrap items-center gap-2">
        <button className="btn-ghost" disabled={busy} onClick={() => void run()}>
          <Cpu size={14} /> Optimize {series ? "all episodes" : "movie"}
        </button>
        {msg && (
          <span className="text-xs text-content-muted">
            {msg.includes("Activity") ? (
              <>
                {msg.split("Activity")[0]}
                <Link to="/activity" className="text-accent hover:underline">
                  Activity
                </Link>
                {msg.split("Activity")[1]}
              </>
            ) : (
              msg
            )}
          </span>
        )}
      </div>
    </section>
  );
}

function DeleteItem({ row, onDeleted }: { row: ManageRow; onDeleted: (r: DeleteResult) => void }) {
  const [confirm, setConfirm] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  useEffect(() => setConfirm(false), [row.id]);
  const what = row.kind === "series" ? "series" : "movie";
  const run = async () => {
    setBusy(true);
    setErr(null);
    try {
      onDeleted(await api<DeleteResult>(`/api/items/${row.id}`, { method: "DELETE" }));
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
      setConfirm(false);
    }
  };
  return (
    <section className="panel-section">
      <div className="card-title mb-2">Delete</div>
      <p className="text-xs text-content-muted">
        Deletes {row.kind === "series" ? "every episode" : "every copy"} of this {what} from the NAS: {row.fileCount} file{row.fileCount === 1 ? "" : "s"},{" "}
        {fmtBytes(row.size)}, and matching subtitle files. Empty folders are removed. This can't be undone.
      </p>
      {err && (
        <div className="mt-2">
          <ErrorNote>{err}</ErrorNote>
        </div>
      )}
      <div className="mt-3 flex gap-2">
        <button className={confirm ? "btn-danger" : "btn-ghost hover:!border-critical hover:!text-critical"} disabled={busy} onClick={() => (confirm ? void run() : setConfirm(true))}>
          <Trash2 size={14} /> {confirm ? `Yes, delete the whole ${what}` : `Delete ${what}`}
        </button>
        {confirm && (
          <button className="btn-quiet" onClick={() => setConfirm(false)}>
            Cancel
          </button>
        )}
      </div>
    </section>
  );
}
