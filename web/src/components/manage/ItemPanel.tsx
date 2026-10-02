import { useEffect, useState } from "react";
import { Cpu, ExternalLink, MoreHorizontal, RotateCcw, Scissors, Trash2, X } from "lucide-react";
import Link from "../Link";
import { PosterArt } from "../PosterCard";
import { ErrorNote, JobNote, MenuButton, type MenuItem } from "../ui";
import ArtworkEditor from "./ArtworkEditor";
import FileList from "./FileList";
import MetadataSearch from "./MetadataSearch";
import { api, useApi } from "../../lib/api";
import { fmtBytes, relPath } from "../../lib/format";
import type { DeleteResult, Library, ManageFile, ManageRow } from "../../lib/types";
import { useStatus } from "../../stores/status";

/** A message at the top of the panel: what a delete or a queued job did. */
type Note = { text: string; error?: boolean };

/** Everything you can do to one movie or show, in a drawer over the Manage table. */
export default function ItemPanel({ row, library, onClose, onChanged }: { row: ManageRow; library: Library; onClose: () => void; onChanged: () => void }) {
  const { data: files, reload: reloadFiles } = useApi<ManageFile[]>(`/api/items/${row.id}/files`);
  const [note, setNote] = useState<Note | null>(null);

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
    setNote({ text: parts.join(" ") });
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
          <ItemActions row={row} onNote={setNote} />
          <button className="btn-quiet" onClick={onClose} aria-label="Close">
            <X size={16} />
          </button>
        </div>
        <div className="flex-1 overflow-y-auto">
          {note && (
            <div className="px-4 pb-3">
              {note.error ? (
                <ErrorNote>{note.text}</ErrorNote>
              ) : (
                <div className="tint-good rounded-md px-3 py-2 text-xs">
                  <JobNote msg={note.text} className="" />
                </div>
              )}
            </div>
          )}
          <MetadataSearch row={row} onMatched={onChanged} />
          <ArtworkEditor row={row} onChanged={onChanged} />
          <FileList kind={row.kind} files={files} libraryPath={library.path} onDeleted={deleted} onChanged={changed} />
          <DeleteItem row={row} onDeleted={deleted} />
        </div>
      </aside>
    </>
  );
}

/**
 * Background jobs for one title, behind a "⋯" in the panel header: optimize
 * copies, and commercial detection when comskip is installed.
 */
function ItemActions({ row, onNote }: { row: ManageRow; onNote: (n: Note) => void }) {
  const comskip = useStatus((s) => s.status?.comskip);
  const series = row.kind === "series";
  const what = series ? "episode" : "file";
  const run = async (path: string, json: unknown, done: (queued: number) => string) => {
    try {
      const r = await api<{ queued: number }>(path, { method: "POST", json });
      onNote({ text: done(r?.queued ?? 0) });
      void useStatus.getState().refresh();
    } catch (e) {
      onNote({ text: e instanceof Error ? e.message : String(e), error: true });
    }
  };
  const optimize = () =>
    run(`/api/items/${row.id}/optimize`, undefined, (n) =>
      n
        ? `Queued ${n} encode${n === 1 ? "" : "s"}; progress is on the Activity page.`
        : series
          ? "Every episode already plays in the browser or has an optimized copy."
          : "This already plays in the browser or has an optimized copy.",
    );
  const commercials = (redo: boolean) =>
    run(`/api/items/${row.id}/commercials`, { redo }, (n) =>
      n
        ? `Looking for commercials in ${n} ${what}${n === 1 ? "" : "s"}; progress is on the Activity page.`
        : series
          ? "Every episode has already been checked for commercials."
          : "This has already been checked for commercials.",
    );
  const items: MenuItem[] = [
    {
      id: "optimize",
      label: series ? "Optimize every episode" : "Optimize",
      detail: "Encode browser-friendly H.264 copies of files that can't play directly (about 1–4 GB per movie at 1080p).",
      icon: <Cpu size={15} />,
      onSelect: () => void optimize(),
    },
  ];
  if (comskip)
    items.push(
      {
        id: "commercials",
        label: series ? "Find commercials in every episode" : "Find commercials",
        detail: series ? "Checks episodes that haven't been checked yet." : "Runs if it hasn't been checked yet.",
        icon: <Scissors size={15} />,
        onSelect: () => void commercials(false),
      },
      {
        id: "commercials-redo",
        label: series ? "Check every episode again" : "Check for commercials again",
        detail: series ? "Also re-checks episodes already done." : "Replaces the breaks found before.",
        icon: <RotateCcw size={15} />,
        onSelect: () => void commercials(true),
      },
    );
  return <MenuButton label="Actions" icon={<MoreHorizontal size={16} />} items={items} align="end" className="btn-quiet" />;
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
