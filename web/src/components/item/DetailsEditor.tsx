import { useEffect, useState } from "react";
import { Check } from "lucide-react";
import { ErrorNote, Spinner } from "../ui";
import { api } from "../../lib/api";
import { errText } from "../../lib/errors";
import type { Item } from "../../lib/types";

const RATINGS = ["G", "PG", "PG-13", "R", "NC-17", "TV-Y", "TV-Y7", "TV-G", "TV-PG", "TV-14", "TV-MA", "NR"];

/**
 * A title's details by hand: title, year, content rating, genres and
 * description, as a section of the Edit panel. A field left empty uses the
 * provider's value (shown as the placeholder); what's filled in stays
 * through every re-match.
 */
export default function DetailsEditor({ item, onSaved }: { item: Item; onSaved: () => void }) {
  const o = item.overrides ?? {};
  const [title, setTitle] = useState(o.title ?? "");
  const [year, setYear] = useState(o.year ? String(o.year) : "");
  const [rated, setRated] = useState(o.rated ?? "");
  const [genres, setGenres] = useState(o.genres?.join(", ") ?? "");
  const [plot, setPlot] = useState(o.plot ?? "");
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  useEffect(() => {
    setTitle(o.title ?? "");
    setYear(o.year ? String(o.year) : "");
    setRated(o.rated ?? "");
    setGenres(o.genres?.join(", ") ?? "");
    setPlot(o.plot ?? "");
    setSaved(false);
  }, [item.id, o.title, o.year, o.rated, o.genres, o.plot]);

  const dirty =
    title !== (o.title ?? "") || year !== (o.year ? String(o.year) : "") || rated !== (o.rated ?? "") || genres !== (o.genres?.join(", ") ?? "") || plot !== (o.plot ?? "");

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
      setSaved(true);
      onSaved();
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(false);
    }
  };
  const providerHint = item.matchProvider ? `Empty fields use what ${item.matchProvider === "tmdb" ? "TMDB" : "the provider"} says.` : "Empty fields use the file name's title and year.";

  return (
    <section className="panel-section">
      <div className="card-title mb-1">Details</div>
      <p className="mb-3 text-xs text-content-muted">{providerHint} What you fill in stays, even when the title is matched again.</p>
      <div className="grid gap-3 sm:grid-cols-[1fr_6rem]">
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
        <textarea className="field min-h-24 w-full" value={plot} onChange={(e) => setPlot(e.target.value)} placeholder={o.plot ? "" : item.plot} maxLength={5000} />
      </label>
      {err && (
        <div className="mt-3">
          <ErrorNote>{err}</ErrorNote>
        </div>
      )}
      <div className="mt-3 flex items-center gap-2">
        <button className="btn-primary" disabled={busy || !dirty} onClick={() => void save()}>
          {busy ? <Spinner size={14} /> : <Check size={14} />} Save details
        </button>
        {saved && !dirty && <span className="text-xs text-content-muted">Saved.</span>}
      </div>
    </section>
  );
}
