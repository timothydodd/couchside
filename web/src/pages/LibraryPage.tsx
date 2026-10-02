import { useMemo, useState } from "react";
import { Clapperboard, Tv, X } from "lucide-react";
import Link from "../components/Link";
import PosterGrid from "../components/PosterGrid";
import FilterMenu, { Choices } from "../components/FilterMenu";
import { EmptyState, ErrorNote, SearchInput, Spinner } from "../components/ui";
import { useApi } from "../lib/api";
import type { ItemKind, ItemSummary } from "../lib/types";

type Sort = "title" | "added" | "year" | "rating";
type Filter = "all" | "unwatched" | "watched" | "unmatched";

const SORTS: Record<Sort, (a: ItemSummary, b: ItemSummary) => number> = {
  title: (a, b) => a.sortTitle.localeCompare(b.sortTitle),
  added: (a, b) => b.lastAddedAt - a.lastAddedAt,
  year: (a, b) => (b.year ?? 0) - (a.year ?? 0) || a.sortTitle.localeCompare(b.sortTitle),
  rating: (a, b) => (b.rating ?? -1) - (a.rating ?? -1) || a.sortTitle.localeCompare(b.sortTitle),
};

const FILTER_LABELS: Record<Filter, string> = { all: "All", unwatched: "Unwatched", watched: "Watched", unmatched: "Unmatched" };

/** An active filter under the search box; tap to clear it. */
function ActiveChip({ label, onClear }: { label: string; onClear: () => void }) {
  return (
    <button type="button" onClick={onClear} className="choice choice-on inline-flex items-center gap-1 !min-h-7 !text-xs" aria-label={`Clear ${label}`}>
      {label}
      <X size={12} />
    </button>
  );
}

// Keep filter choices per library view across navigation.
const saved: Record<string, { q: string; sort: Sort; filter: Filter; genre: string }> = {};

export default function LibraryPage({ kind }: { kind: ItemKind }) {
  const { data, error, loading } = useApi<ItemSummary[]>(`/api/items?kind=${kind}`, { pollMs: 20000 });
  const init = saved[kind] ?? { q: "", sort: "title", filter: "all", genre: "" };
  const [q, setQ] = useState(init.q);
  const [sort, setSort] = useState<Sort>(init.sort);
  const [filter, setFilter] = useState<Filter>(init.filter);
  const [genre, setGenre] = useState(init.genre);
  saved[kind] = { q, sort, filter, genre };

  const genres = useMemo(() => [...new Set((data ?? []).flatMap((i) => i.genres))].sort(), [data]);

  const items = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return (data ?? [])
      .filter((i) => !needle || i.title.toLowerCase().includes(needle))
      .filter((i) => !genre || i.genres.includes(genre))
      .filter((i) => {
        const watched = i.fileCount > 0 && i.watchedCount >= i.fileCount;
        if (filter === "unwatched") return !watched;
        if (filter === "watched") return watched;
        if (filter === "unmatched") return i.matchStatus === "unmatched";
        return true;
      })
      .sort(SORTS[sort]);
  }, [data, q, sort, filter, genre]);

  const title = kind === "movie" ? "Movies" : "TV Shows";
  const noun = kind === "movie" ? "movie" : "show";
  const total = data?.length ?? 0;

  return (
    <div className="flex h-full flex-col">
      <header className="gutter border-b border-border-light pb-3 pt-4 md:pt-5">
        <div className="flex items-baseline justify-between gap-3">
          <h1 className="text-lg font-semibold text-content">{title}</h1>
          <span className="text-xs text-content-muted">
            {data
              ? items.length === total
                ? `${total.toLocaleString()} ${noun}${total === 1 ? "" : "s"}`
                : `${items.length.toLocaleString()} of ${total.toLocaleString()} ${noun}s`
              : ""}
          </span>
        </div>
        <div className="mt-3 flex items-center gap-2">
          <SearchInput large value={q} onChange={setQ} placeholder={`Search ${noun}s`} aria-label={`Search ${noun}s`} className="min-w-0 flex-1 md:max-w-xl" />
          <FilterMenu
            active={(filter !== "all" ? 1 : 0) + (genre ? 1 : 0) + (sort !== "title" ? 1 : 0)}
            onReset={() => {
              setFilter("all");
              setGenre("");
              setSort("title");
            }}
          >
            <Choices<Sort>
              label="Sort by"
              value={sort}
              onChange={setSort}
              options={[
                { id: "title", label: "Title" },
                { id: "added", label: "Recently added" },
                { id: "year", label: "Year" },
                { id: "rating", label: "Rating" },
              ]}
            />
            <Choices<Filter>
              label="Show"
              value={filter}
              onChange={setFilter}
              options={[
                { id: "all", label: "All" },
                { id: "unwatched", label: "Unwatched" },
                { id: "watched", label: "Watched" },
                { id: "unmatched", label: "Unmatched" },
              ]}
            />
            {genres.length > 0 && (
              <Choices label="Genre" value={genre} onChange={setGenre} options={[{ id: "", label: "All genres" }, ...genres.map((g) => ({ id: g, label: g }))]} />
            )}
          </FilterMenu>
        </div>
        {(filter !== "all" || genre) && (
          <div className="mt-2 flex flex-wrap gap-1.5">
            {filter !== "all" && <ActiveChip label={FILTER_LABELS[filter]} onClear={() => setFilter("all")} />}
            {genre && <ActiveChip label={genre} onClear={() => setGenre("")} />}
          </div>
        )}
      </header>
      {error && (
        <div className="gutter pt-4">
          <ErrorNote>{error}</ErrorNote>
        </div>
      )}
      {loading && !data ? (
        <div className="flex flex-1 items-center justify-center">
          <Spinner size={22} />
        </div>
      ) : total === 0 ? (
        <EmptyState icon={kind === "movie" ? <Clapperboard size={36} strokeWidth={1.5} /> : <Tv size={36} strokeWidth={1.5} />} title={`No ${noun}s yet`}>
          Add a {kind === "movie" ? "Movies" : "TV"} library on the <Link to="/libraries" className="text-accent hover:underline">Libraries</Link> page, or
          wait for the current scan to finish.
        </EmptyState>
      ) : items.length === 0 ? (
        <EmptyState title="Nothing matches those filters" />
      ) : (
        <PosterGrid items={items} memoryKey={kind} />
      )}
    </div>
  );
}
