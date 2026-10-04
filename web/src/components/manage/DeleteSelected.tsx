import { useRef, useState } from "react";
import { Trash2 } from "lucide-react";
import { ErrorNote } from "../ui";
import { api } from "../../lib/api";
import { useDialog } from "../../lib/dialog";
import { errText } from "../../lib/errors";
import { fmtBytes } from "../../lib/format";
import { notify } from "../../lib/notices";
import type { DeleteResult, ItemSummary } from "../../lib/types";

const SHOWN = 8; // titles listed before "and N more"

/**
 * Confirms, then deletes the selected titles' files from disk, one title at
 * a time. It stops at the first title that can't be deleted and says which;
 * the ones before it are gone.
 */
export default function DeleteSelected({ items, onClose, onDeleted }: { items: ItemSummary[]; onClose: () => void; onDeleted: () => void }) {
  const [done, setDone] = useState<number | null>(null); // titles deleted so far, while running
  const [err, setErr] = useState<string | null>(null);
  const busy = done !== null;
  const dialog = useRef<HTMLDivElement>(null);
  const close = () => !busy && onClose();
  useDialog(dialog, close);

  const n = items.length;
  const what = n === 1 ? "title" : "titles";
  const run = async () => {
    setErr(null);
    let files = 0;
    let bytes = 0;
    for (let i = 0; i < n; i++) {
      setDone(i);
      try {
        const r = await api<DeleteResult>(`/api/items/${items[i].id}`, { method: "DELETE" });
        files += r.deleted;
        bytes += r.bytes;
      } catch (e) {
        setDone(null);
        setErr(`${items[i].title}: ${errText(e)}${i > 0 ? ` (${i} of ${n} were deleted before it.)` : ""}`);
        if (i > 0) onDeleted();
        return;
      }
    }
    notify(`Deleted ${n} ${what}: ${files} file${files === 1 ? "" : "s"} (${fmtBytes(bytes)}).`, "info");
    onDeleted();
    onClose();
  };

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-backdrop/55 p-4" onClick={close}>
      <div className="card w-full max-w-md p-4 shadow-[var(--shadow-md)]" onClick={(e) => e.stopPropagation()} role="dialog" aria-modal="true" aria-label={`Delete ${n} ${what}`} ref={dialog}>
        <div className="text-base font-semibold text-content">
          Delete {n} {what}?
        </div>
        <ul className="mt-2 max-h-48 overflow-y-auto text-sm text-content-secondary">
          {items.slice(0, SHOWN).map((it) => (
            <li key={it.id} className="truncate">
              {it.title}
              {it.year ? ` (${it.year})` : ""}
            </li>
          ))}
          {n > SHOWN && <li className="text-content-muted">and {n - SHOWN} more</li>}
        </ul>
        <p className="mt-3 text-xs text-content-muted">
          Deletes every file of {n === 1 ? "this title" : "these titles"} from the NAS (every episode of a show, every copy of a movie), and matching subtitle
          files. Empty folders are removed. This can't be undone.
        </p>
        {err && (
          <div className="mt-3">
            <ErrorNote>{err}</ErrorNote>
          </div>
        )}
        <div className="mt-4 flex justify-end gap-2">
          <button className="btn-quiet" disabled={busy} onClick={onClose}>
            Cancel
          </button>
          <button className="btn-danger" disabled={busy} onClick={() => void run()}>
            <Trash2 size={14} /> {busy ? `Deleting ${(done ?? 0) + 1} of ${n}…` : `Delete ${n} ${what}`}
          </button>
        </div>
      </div>
    </div>
  );
}
