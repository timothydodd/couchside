import { useRef, useState } from "react";
import { Combine } from "lucide-react";
import { ErrorNote } from "../ui";
import { api } from "../../lib/api";
import { useDialog } from "../../lib/dialog";
import { errText } from "../../lib/errors";
import type { ItemSummary } from "../../lib/types";

/**
 * Merge the selected titles into one: pick which stays, and for movies what
 * the others' files become (duplicate copies of the same film, or bonus
 * material). Their watch history and lists carry over; nothing on disk moves.
 */
export default function MergeTitles({ items, onClose, onMerged }: { items: ItemSummary[]; onClose: () => void; onMerged: () => void }) {
  const movies = items[0]?.kind === "movie";
  // Default to the one with metadata and the most files: the best-known copy of the title.
  const best = [...items].sort((a, b) => Number(b.matchStatus === "matched") - Number(a.matchStatus === "matched") || b.fileCount - a.fileCount)[0];
  const [into, setInto] = useState(best?.id ?? 0);
  const [as, setAs] = useState<"copy" | "extra">("copy");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const dialog = useRef<HTMLDivElement>(null);
  const close = () => !busy && onClose();
  useDialog(dialog, close);

  const run = async () => {
    setBusy(true);
    setErr(null);
    try {
      await api("/api/items/merge", { method: "POST", json: { into, from: items.filter((i) => i.id !== into).map((i) => i.id), as } });
      onMerged();
      onClose();
    } catch (e) {
      setErr(errText(e));
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-backdrop/55 p-4" onClick={close}>
      <div className="card w-full max-w-md p-4 shadow-[var(--shadow-md)]" onClick={(e) => e.stopPropagation()} role="dialog" aria-modal="true" aria-label="Merge titles" ref={dialog}>
        <div className="text-base font-semibold text-content">Merge {items.length} titles into one</div>
        <div className="mt-3 field-label">Keep</div>
        <div className="mt-1 flex max-h-56 flex-col gap-1 overflow-y-auto" role="radiogroup" aria-label="Title to keep">
          {items.map((it) => (
            <label key={it.id} className="flex items-center gap-2 text-sm text-content">
              <input type="radio" name="into" className="accent-brand" checked={into === it.id} onChange={() => setInto(it.id)} />
              <span className="truncate">
                {it.title}
                {it.year ? ` (${it.year})` : ""}
              </span>
              <span className="ml-auto shrink-0 text-xs text-content-muted">
                {it.fileCount} file{it.fileCount === 1 ? "" : "s"}
                {it.matchStatus !== "matched" ? " · unmatched" : ""}
              </span>
            </label>
          ))}
        </div>
        {movies ? (
          <>
            <div className="mt-3 field-label">The other films' files are</div>
            <div className="mt-1 flex flex-col gap-1" role="radiogroup" aria-label="What the other files become">
              <label className="flex items-start gap-2 text-sm text-content">
                <input type="radio" name="as" className="mt-1 accent-brand" checked={as === "copy"} onChange={() => setAs("copy")} />
                <span>
                  Duplicates
                  <span className="block text-xs text-content-muted">The same film: shown as extra copies to pick between.</span>
                </span>
              </label>
              <label className="flex items-start gap-2 text-sm text-content">
                <input type="radio" name="as" className="mt-1 accent-brand" checked={as === "extra"} onChange={() => setAs("extra")} />
                <span>
                  Bonus material
                  <span className="block text-xs text-content-muted">Featurettes, deleted scenes: listed under the film, named after the title they were.</span>
                </span>
              </label>
            </div>
          </>
        ) : (
          <p className="mt-3 text-xs text-content-muted">The other shows' episodes join this one by season and number. Watch history comes along.</p>
        )}
        <p className="mt-3 text-xs text-content-muted">Nothing on disk moves. Scans leave merged files where you put them; the other titles disappear from the library.</p>
        {err && (
          <div className="mt-3">
            <ErrorNote>{err}</ErrorNote>
          </div>
        )}
        <div className="mt-4 flex justify-end gap-2">
          <button className="btn-quiet" disabled={busy} onClick={onClose}>
            Cancel
          </button>
          <button className="btn-primary" disabled={busy || !into} onClick={() => void run()}>
            <Combine size={14} /> {busy ? "Merging…" : "Merge"}
          </button>
        </div>
      </div>
    </div>
  );
}
