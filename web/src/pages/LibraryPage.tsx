import { useMemo, useState } from "react";
import { Clapperboard, Tv } from "lucide-react";
import Link from "../components/Link";
import PosterGrid from "../components/PosterGrid";
import { EmptyState, ErrorNote, PageHeader, SearchInput, Spinner } from "../components/ui";
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
      <PageHeader
        title={title}
        subtitle={
          data
            ? items.length === total
              ? `${total.toLocaleString()} ${noun}${total === 1 ? "" : "s"}`
              : `${items.length.toLocaleString()} of ${total.toLocaleString()} ${noun}s`
            : " "
        }
      >
        <SearchInput value={q} onChange={setQ} placeholder={`Search ${noun}s…`} className="w-56" />
        {genres.length > 0 && (
          <select className="field" value={genre} onChange={(e) => setGenre(e.target.value)} aria-label="Genre">
            <option value="">All genres</option>
            {genres.map((g) => (
              <option key={g}>{g}</option>
            ))}
          </select>
        )}
        <select className="field" value={filter} onChange={(e) => setFilter(e.target.value as Filter)} aria-label="Filter">
          <option value="all">All</option>
          <option value="unwatched">Unwatched</option>
          <option value="watched">Watched</option>
          <option value="unmatched">Unmatched</option>
        </select>
        <select className="field" value={sort} onChange={(e) => setSort(e.target.value as Sort)} aria-label="Sort">
          <option value="title">Title</option>
          <option value="added">Recently added</option>
          <option value="year">Year</option>
          <option value="rating">Rating</option>
        </select>
      </PageHeader>
      {error && (
        <div className="px-6 pt-4">
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
