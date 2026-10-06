import { useRef, useState } from "react";
import { Pencil } from "lucide-react";
import { ErrorNote } from "../ui";
import { api } from "../../lib/api";
import { useDialog } from "../../lib/dialog";
import { errText } from "../../lib/errors";
import type { Item } from "../../lib/types";

const RATINGS = ["G", "PG", "PG-13", "R", "NC-17", "TV-Y", "TV-Y7", "TV-G", "TV-PG", "TV-14", "TV-MA", "NR"];

/**
 * Change a title's details by hand: title, year, content rating, genres and
 * description. A field left empty uses the provider's value (shown as the
 * placeholder); what's filled in stays through every re-match.
 */
export default function EditDetails({ item, onClose, onSaved }: { item: Item; onClose: () => void; onSaved: () => void }) {
  const o = item.overrides ?? {};
  const [title, setTitle] = useState(o.title ?? "");
  const [year, setYear] = useState(o.year ? String(o.year) : "");
  const [rated, setRated] = useState(o.rated ?? "");
  const [genres, setGenres] = useState(o.genres?.join(", ") ?? "");
  const [plot, setPlot] = useState(o.plot ?? "");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const dialog = useRef<HTMLDivElement>(null);
  const close = () => !busy && onClose();
  useDialog(dialog, close);

  const save = async () => {
    setBusy(true);
    setErr(null);
    try {
      await api(`/api/items/${item.id}/details`, {
        method: "PUT",
        json: {
          title: title.trim() || undefined,
          year: year.trim() ? Number(year) : undefined,
          rated: rated.trim() || undefined,
          genres: genres.trim() ? genres.split(",").map((g) => g.trim()).filter(Boolean) : undefined,
          plot: plot.trim() || undefined,
        },
      });
      onSaved();
      onClose();
    } catch (e) {
      setErr(errText(e));
      setBusy(false);
    }
  };
  const providerHint = item.matchProvider ? `Empty fields use what ${item.matchProvider === "tmdb" ? "TMDB" : "the provider"} says.` : "Empty fields use the file name's title and year.";

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-backdrop/55 p-4" onClick={close}>
      <div className="card w-full max-w-lg p-4 shadow-[var(--shadow-md)]" onClick={(e) => e.stopPropagation()} role="dialog" aria-modal="true" aria-label={`Edit details of ${item.title}`} ref={dialog}>
        <div className="text-base font-semibold text-content">Edit details</div>
        <p className="mt-1 text-xs text-content-muted">{providerHint} What you fill in stays, even when the title is matched again.</p>
        <div className="mt-3 grid gap-3 sm:grid-cols-[1fr_6rem]">
          <label className="block">
            <span className="field-label">Title</span>
            <input className="field w-full" value={title} onChange={(e) => setTitle(e.target.value)} placeholder={o.title ? "" : item.title} maxLength={200} />
          </label>
          <label className="block">
            <span className="field-label">Year</span>
            <input className="field w-full" inputMode="numeric" value={year} onChange={(e) => setYear(e.target.value.replace(/\D/g, "").slice(0, 4))} placeholder={item.year ? String(item.year) : ""} />
          </label>
        </div>
        <div className="mt-3 grid gap-3 sm:grid-cols-[8rem_1fr]">
          <label className="block">
            <span className="field-label">Rated</span>
            <input className="field w-full" list="ratings" value={rated} onChange={(e) => setRated(e.target.value)} placeholder={o.rated ? "" : item.rated || "e.g. PG"} maxLength={10} />
            <datalist id="ratings">
              {RATINGS.map((r) => (
                <option key={r} value={r} />
              ))}
            </datalist>
          </label>
          <label className="block">
            <span className="field-label">Genres</span>
            <input className="field w-full" value={genres} onChange={(e) => setGenres(e.target.value)} placeholder={o.genres ? "" : item.genres.join(", ") || "Comedy, Drama"} />
          </label>
        </div>
        <label className="mt-3 block">
          <span className="field-label">Description</span>
          <textarea className="field min-h-28 w-full" value={plot} onChange={(e) => setPlot(e.target.value)} placeholder={o.plot ? "" : item.plot} maxLength={5000} />
        </label>
        {err && (
          <div className="mt-3">
            <ErrorNote>{err}</ErrorNote>
          </div>
        )}
        <div className="mt-4 flex justify-end gap-2">
          <button className="btn-quiet" disabled={busy} onClick={onClose}>
            Cancel
          </button>
          <button className="btn-primary" disabled={busy} onClick={() => void save()}>
            <Pencil size={14} /> {busy ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}
