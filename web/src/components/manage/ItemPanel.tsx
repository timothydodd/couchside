import { useEffect, useRef, useState } from "react";
import { X } from "lucide-react";
import Link from "../Link";
import { PosterArt } from "../PosterCard";
import { ErrorNote, JobNote, Spinner } from "../ui";
import ArtworkEditor from "./ArtworkEditor";
import FileList from "./FileList";
import MetadataSearch from "./MetadataSearch";
import DetailsEditor from "../item/DetailsEditor";
import { useApi } from "../../lib/api";
import { fmtBytes, relPath } from "../../lib/format";
import type { DeleteResult, Item, Library, ManageFile, ManageRow } from "../../lib/types";
import { useRouter } from "../../stores/router";
import { useDialog } from "../../lib/dialog";

/** A message at the top of the panel: what a delete did. */
type Note = { text: string; error?: boolean };

export type PanelSection = "details" | "match" | "artwork" | "files";

/**
 * Everything an admin can change about one movie or show, in a drawer over
 * its page: details by hand, the match, the artwork, and its files (roles,
 * extra copies, deleting). Jobs and deleting the whole title stay in the
 * page's "⋯". With `episode`, only that episode's files are listed; the
 * rest belongs to the show.
 */
export default function ItemPanel({
  item,
  episode,
  section,
  onClose,
  onChanged,
}: {
  item: Item;
  /** Scope the panel to one episode: its number for the file list, and how the header names it. */
  episode?: { season: number; episode: number; label: string };
  /** The section to scroll to on open. */
  section?: PanelSection;
  onClose: () => void;
  onChanged: () => void;
}) {
  const { data, error, reload: reloadRow } = useApi<{ item: ManageRow; library: Library }>(`/api/items/${item.id}/manage`);
  const { data: files, error: filesError, reload: reloadFiles } = useApi<ManageFile[]>(`/api/items/${item.id}/files`);
  const [note, setNote] = useState<Note | null>(null);
  const go = useRouter((s) => s.go);

  const dialog = useRef<HTMLElement>(null);
  useDialog(dialog, onClose);
  useEffect(() => setNote(null), [item.id]);
  // Open on the section asked for, once the row has loaded and it's rendered.
  const scrolled = useRef(false);
  useEffect(() => {
    if (!section || scrolled.current || !data) return;
    scrolled.current = true;
    dialog.current?.querySelector(`[data-section="${section}"]`)?.scrollIntoView({ block: "start" });
  }, [section, data]);

  const changed = () => {
    void reloadRow();
    void reloadFiles();
    onChanged();
  };
  const deleted = (r: DeleteResult) => {
    const parts = [`Deleted ${r.deleted} file${r.deleted === 1 ? "" : "s"} (${fmtBytes(r.bytes)}).`];
    if (r.keptFolders.length && data) parts.push(`Left ${r.keptFolders.map((f) => relPath(f, data.library.path)).join(", ")}: it still holds other files (artwork, .nfo).`);
    setNote({ text: parts.join(" ") });
    if (r.itemsRemoved.includes(item.id)) {
      onClose();
      go(item.kind === "series" ? "/tv" : "/movies", { replace: true });
      return;
    }
    changed();
  };

  const row = data?.item;
  const shown = episode ? files?.filter((f) => f.season === episode.season && f.episode === episode.episode) : files;
  const what = episode ? episode.label : item.title;

  return (
    <>
      <div className="fixed inset-0 z-30 bg-backdrop/30" onClick={onClose} />
      <aside className="side-panel" role="dialog" aria-modal="true" aria-label={`Edit ${what}`} ref={dialog}>
        <div className="flex items-start gap-3 px-4 py-4">
          <div className="poster relative w-12 shrink-0">
            <PosterArt item={item} bare />
          </div>
          <div className="min-w-0 flex-1">
            <div className="text-xs text-content-muted">Edit</div>
            <div className="truncate text-base font-semibold leading-tight text-content">{what}</div>
            {row && !episode && (
              <div className="mt-0.5 text-xs text-content-muted">
                {[row.year, row.kind === "series" ? `${row.episodeCount} episodes` : null, `${row.fileCount} file${row.fileCount === 1 ? "" : "s"}`, fmtBytes(row.size)]
                  .filter(Boolean)
                  .join(" · ")}
              </div>
            )}
          </div>
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
          {error && (
            <div className="px-4 pb-3">
              <ErrorNote>{error}</ErrorNote>
            </div>
          )}
          {!data && !error && (
            <div className="flex justify-center py-10">
              <Spinner size={20} />
            </div>
          )}
          {row && episode && (
            <section className="panel-section">
              <p className="text-xs text-content-muted">
                The details, match and artwork belong to the show.{" "}
                <Link to={`/item/${item.id}?edit=1`} className="text-content-secondary hover:text-accent">
                  Edit {item.title}
                </Link>
              </p>
            </section>
          )}
          {row && !episode && (
            <>
              <div data-section="details">
                <DetailsEditor item={item} onSaved={changed} />
              </div>
              <div data-section="match">
                <MetadataSearch row={row} onMatched={changed} />
              </div>
              <div data-section="artwork">
                <ArtworkEditor row={row} onChanged={changed} />
              </div>
            </>
          )}
          {row && data && (
            <div data-section="files">
              <FileList kind={row.kind} files={shown} error={filesError} libraryPath={data.library.path} onDeleted={deleted} onChanged={changed} />
            </div>
          )}
        </div>
      </aside>
    </>
  );
}
