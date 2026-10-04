import { useEffect, useState } from "react";
import { Check, Search } from "lucide-react";
import { ErrorNote, Spinner } from "../ui";
import { api } from "../../lib/api";
import { titleLink } from "../../lib/format";
import type { ManageRow, SearchResult } from "../../lib/types";
import { useStatus } from "../../stores/status";
import { errText } from "../../lib/errors";

/**
 * Fix a match by searching TMDB (or OMDb) under any name, or pasting an IMDb
 * or TMDB id or link, and picking the right result. The pick is pinned, so
 * rescans keep it.
 */
export default function MetadataSearch({ row, onMatched }: { row: ManageRow; onMatched: () => void }) {
  const hasProvider = !!useStatus((s) => s.status?.providers.length);
  const [q, setQ] = useState(row.parsedTitle);
  const [year, setYear] = useState(row.parsedYear ? String(row.parsedYear) : "");
  const [results, setResults] = useState<SearchResult[] | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    setQ(row.parsedTitle);
    setYear(row.parsedYear ? String(row.parsedYear) : "");
    setResults(null);
    setErr(null);
  }, [row.id, row.parsedTitle, row.parsedYear]);

  const search = async () => {
    setBusy("search");
    setErr(null);
    try {
      const p = new URLSearchParams({ q });
      if (year.trim()) p.set("year", year.trim());
      setResults(await api<SearchResult[]>(`/api/items/${row.id}/lookup?${p}`));
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(null);
    }
  };
  const use = async (r: SearchResult) => {
    setBusy(r.imdbId);
    setErr(null);
    try {
      await api(`/api/items/${row.id}/match`, { method: "POST", json: { imdbId: r.imdbId } });
      setResults(null);
      onMatched();
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(null);
    }
  };

  return (
    <section className="panel-section">
      <div className="card-title mb-1">Match</div>
      <p className="mb-3 text-xs text-content-muted">
        {row.matchStatus === "unmatched" ? "Not matched to anything yet." : row.matchStatus === "pending" ? "Looking it up…" : "Matched to "}
        {row.imdbId && row.matchStatus !== "unmatched" && (
          <a href={titleLink(row.imdbId) ?? undefined} target="_blank" rel="noreferrer" className="text-content-secondary hover:text-accent">
            {row.imdbId}
          </a>
        )}
        {row.matchStatus === "matched" && ". Wrong? Search under another name and pick the right one."}
      </p>
      {!hasProvider ? (
        <p className="text-xs text-warning">Searching needs a metadata provider: set TMDB_API_KEY (or OMDB_API_KEY) on the server.</p>
      ) : (
        <form
          className="flex gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            void search();
          }}
        >
          <input className="field min-w-0 flex-1" value={q} onChange={(e) => setQ(e.target.value)} placeholder="Title, or an IMDb or TMDB id/link" aria-label="Title to search" />
          <input className="field w-20" value={year} onChange={(e) => setYear(e.target.value.replace(/\D/g, "").slice(0, 4))} placeholder="Year" aria-label="Year" />
          <button className="btn-primary" disabled={!q.trim() || busy !== null}>
            {busy === "search" ? <Spinner size={14} /> : <Search size={14} />}
          </button>
        </form>
      )}
      {err && (
        <div className="mt-2">
          <ErrorNote>{err}</ErrorNote>
        </div>
      )}
      {results && (
        <ul className="mt-3 flex flex-col gap-1.5">
          {results.length === 0 && <li className="text-xs text-content-muted">No results. Try a shorter or different title, or drop the year.</li>}
          {results.map((r) => (
            <li key={r.imdbId} className={`flex items-center gap-3 rounded-md border px-2 py-1.5 ${r.imdbId === row.imdbId ? "border-accent" : "border-border-light"}`}>
              <div className="h-15 w-10 shrink-0 overflow-hidden rounded bg-raised">
                {r.poster && <img src={r.poster} alt="" loading="lazy" className="h-full w-full object-cover" />}
              </div>
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm text-content">{r.title}</div>
                <div className="text-xs text-content-muted">
                  {r.year} ·{" "}
                  <a href={titleLink(r.imdbId) ?? undefined} target="_blank" rel="noreferrer" className="hover:text-accent">
                    {r.imdbId}
                  </a>
                </div>
              </div>
              {r.imdbId === row.imdbId ? (
                <span className="badge tint-good">
                  <Check size={11} /> Current
                </span>
              ) : (
                <button className="btn-ghost !px-2.5 !py-1 !text-xs" disabled={busy !== null} onClick={() => void use(r)}>
                  {busy === r.imdbId ? <Spinner size={12} /> : "Use this"}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
