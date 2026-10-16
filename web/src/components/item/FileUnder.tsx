import { useMemo, useState } from "react";
import { FolderInput } from "lucide-react";
import Modal from "../Modal";
import { ErrorNote, SearchInput } from "../ui";
import { api, useApi } from "../../lib/api";
import { errText } from "../../lib/errors";
import type { Item, ItemSummary } from "../../lib/types";

/**
 * File a movie under another title (a box set, a collection): it leaves the
 * Movies page and is listed on that title's page, still a movie of its own
 * with its own details and watched state. Scans leave the choice alone.
 */
export default function FileUnder({ item, onClose, onFiled }: { item: Item; onClose: () => void; onFiled: () => void }) {
  const { data } = useApi<ItemSummary[]>("/api/items?kind=movie");
  const [q, setQ] = useState("");
  const [parent, setParent] = useState(0);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  // Only titles that can take a child: not this one, and not a child themselves.
  const options = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return (data ?? [])
      .filter((i) => i.id !== item.id && !i.parentId)
      .filter((i) => !needle || i.title.toLowerCase().includes(needle))
      .slice(0, 40);
  }, [data, q, item.id]);

  const run = async () => {
    setBusy(true);
    setErr(null);
    try {
      await api(`/api/items/${item.id}/parent`, { method: "PUT", json: { parentId: parent } });
      onFiled();
    } catch (e) {
      setErr(errText(e));
      setBusy(false);
    }
  };

  return (
    <Modal label="File under" onClose={onClose} busy={busy}>
      <>
        <div className="text-base font-semibold text-content">File {item.title} under…</div>
        <p className="mt-1 text-xs text-content-muted">It leaves the Movies page and is listed on the title you pick, still a movie of its own. Scans leave your choice alone.</p>
        <div className="mt-3">
          <SearchInput value={q} onChange={setQ} placeholder="Find a title…" autoFocus />
        </div>
        <div className="mt-2 flex max-h-64 flex-col gap-1 overflow-y-auto" role="radiogroup" aria-label="Title to file under">
          {options.map((it) => (
            <label key={it.id} className="flex items-center gap-2 text-sm text-content">
              <input type="radio" name="parent" className="accent-brand" checked={parent === it.id} onChange={() => setParent(it.id)} />
              <span className="truncate">
                {it.title}
                {it.year ? ` (${it.year})` : ""}
              </span>
              {it.childCount > 0 && (
                <span className="ml-auto shrink-0 text-xs text-content-muted">
                  {it.childCount} title{it.childCount === 1 ? "" : "s"}
                </span>
              )}
            </label>
          ))}
          {data && options.length === 0 && <div className="py-2 text-sm text-content-muted">No other title matches.</div>}
        </div>
        {err && (
          <div className="mt-3">
            <ErrorNote>{err}</ErrorNote>
          </div>
        )}
        <div className="mt-4 flex justify-end gap-2">
          <button className="btn-quiet" disabled={busy} onClick={onClose}>
            Cancel
          </button>
          <button className="btn-primary" disabled={busy || !parent} onClick={() => void run()}>
            <FolderInput size={14} /> {busy ? "Filing…" : "File under"}
          </button>
        </div>
      </>
    </Modal>
  );
}
