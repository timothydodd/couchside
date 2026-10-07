import { useEffect, useMemo, useRef, useState, type MouseEvent } from "react";
import { Bookmark, BookmarkX, ChevronDown, Clapperboard, Combine, Eye, EyeOff, Play, RefreshCw, Scissors, SearchX, Settings2, Trash2, Tv } from "lucide-react";
import Link from "../components/Link";
import PosterGrid from "../components/PosterGrid";
import DeleteSelected from "../components/manage/DeleteSelected";
import MergeTitles from "../components/manage/MergeTitles";
import FilterMenu, { ActiveChip, Choices } from "../components/FilterMenu";
import { EmptyState, ErrorNote, MenuButton, SearchInput, Loading } from "../components/ui";
import { api, useApi } from "../lib/api";
import { attempt, notify } from "../lib/notices";
import { useIsAdmin } from "../stores/auth";
import { useRouter } from "../stores/router";
import { useQueue, type QueueEntry } from "../stores/queue";
import { useStatus } from "../stores/status";
import type { ItemDetail, ItemKind, ItemSummary } from "../lib/types";

type Sort = "title" | "added" | "year" | "rating";
type Filter = "all" | "unwatched" | "watched" | "list" | "unmatched";

const SORTS: Record<Sort, (a: ItemSummary, b: ItemSummary) => number> = {
  title: (a, b) => a.sortTitle.localeCompare(b.sortTitle),
  added: (a, b) => b.lastAddedAt - a.lastAddedAt,
  year: (a, b) => (b.year ?? 0) - (a.year ?? 0) || a.sortTitle.localeCompare(b.sortTitle),
  rating: (a, b) => (b.rating ?? -1) - (a.rating ?? -1) || a.sortTitle.localeCompare(b.sortTitle),
};

const FILTER_LABELS: Record<Filter, string> = { all: "All", unwatched: "Unwatched", watched: "Watched", list: "My list", unmatched: "Unmatched" };

// Keep filter choices per library view across navigation.
const saved: Record<string, { q: string; sort: Sort; filter: Filter; genre: string }> = {};

export default function LibraryPage({ kind }: { kind: ItemKind }) {
  const { data, error, loading, reload } = useApi<ItemSummary[]>(`/api/items?kind=${kind}`, { pollMs: 20000 });
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
        if (filter === "list") return i.inWatchlist;
        if (filter === "unmatched") return i.matchStatus === "unmatched";
        return true;
      })
      .sort(SORTS[sort]);
  }, [data, q, sort, filter, genre]);

  // Admins pick titles with Ctrl-click (Cmd on a Mac) and Shift-click for a
  // range; while any are picked a plain click picks too. Only picked titles
  // that are in view count, so nothing hidden by a filter is acted on.
  const admin = useIsAdmin();
  const go = useRouter((s) => s.go);
  const [picked, setPicked] = useState<Set<number>>(() => new Set());
  const anchor = useRef<number | null>(null); // the last title clicked, where a Shift-click range starts
  const [deleting, setDeleting] = useState(false);
  const [merging, setMerging] = useState(false);
  const comskip = useStatus((s) => !!s.status?.comskip);
  const chosen = useMemo(() => items.filter((i) => picked.has(i.id)), [items, picked]);
  const clear = () => {
    setPicked(new Set());
    anchor.current = null;
  };
  const pick = (it: ItemSummary, e: MouseEvent<HTMLAnchorElement>) => {
    if (e.button !== 0 || e.altKey) return;
    if (chosen.length === 0 && !e.ctrlKey && !e.metaKey) return; // an ordinary click: open the title
    e.preventDefault();
    const next = new Set(chosen.map((i) => i.id));
    const from = e.shiftKey ? items.findIndex((i) => i.id === anchor.current) : -1;
    if (from >= 0) {
      const to = items.indexOf(it);
      for (const i of items.slice(Math.min(from, to), Math.max(from, to) + 1)) next.add(i.id);
    } else if (!next.delete(it.id)) next.add(it.id);
    anchor.current = it.id;
    setPicked(next);
  };
  const selecting = chosen.length > 0;
  useEffect(() => {
    if (!selecting || deleting) return;
    // Escape clears the selection, unless it's closing the Actions menu.
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && !document.querySelector('[role="menu"]') && clear();
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [selecting, deleting]);
  // Everything playable in the selected titles, in grid order: a movie's
  // chosen copy (or its parts), a show's episodes in order.
  const playAll = attempt("Couldn't start playing", async () => {
    const entries: QueueEntry[] = [];
    for (const it of chosen) {
      const d = await api<ItemDetail & { versionFileId?: number | null }>(`/api/items/${it.id}`);
      if (it.kind === "series") {
        for (const s of d.seasons ?? [])
          for (const e of s.episodes) if (e.fileId) entries.push({ fileId: e.fileId, title: it.title, subtitle: `S${e.season} E${e.episode}${e.title ? ` · ${e.title}` : ""}` });
      } else {
        const year = it.year ? String(it.year) : undefined;
        const parts = d.files.filter((f) => f.role === "part").sort((a, b) => a.partNo - b.partNo);
        if (parts.length) entries.push(...parts.map((f) => ({ fileId: f.id, title: it.title, subtitle: `Part ${f.partNo}` })));
        else if (d.versionFileId) entries.push({ fileId: d.versionFileId, title: it.title, subtitle: year });
        else if (d.files.find((f) => f.role === "copy")) entries.push({ fileId: d.files.find((f) => f.role === "copy")!.id, title: it.title, subtitle: year });
      }
    }
    if (!entries.length) throw new Error("nothing to play in the selection");
    useQueue.getState().start(entries);
    go(`/play/${entries[0].fileId}`);
  });
  const allListed = chosen.length > 0 && chosen.every((i) => i.inWatchlist);
  const setListed = (on: boolean) =>
    attempt(on ? "Couldn't add to My list" : "Couldn't remove from My list", async () => {
      for (const it of chosen) if (it.inWatchlist !== on) await api(`/api/items/${it.id}/watchlist`, { method: on ? "PUT" : "DELETE" });
      await reload();
    })();
  const refreshMeta = attempt("Couldn't refresh metadata", async () => {
    for (const it of chosen) await api(`/api/items/${it.id}/match`, { method: "POST", json: { refresh: true } });
    notify(`Refreshing metadata for ${chosen.length} ${chosen.length === 1 ? "title" : "titles"}; see Activity.`, "info");
    clear();
  });
  const findCommercials = attempt("Couldn't queue commercial detection", async () => {
    for (const it of chosen) await api(`/api/items/${it.id}/commercials`, { method: "POST", json: { redo: false } });
    notify(`Looking for commercials in ${chosen.length} ${chosen.length === 1 ? "title" : "titles"}; see Activity.`, "info");
    clear();
  });
  const setWatched = (watched: boolean) =>
    attempt(`Couldn't mark ${watched ? "watched" : "unwatched"}`, async () => {
      for (const it of chosen) await api(`/api/items/${it.id}/watched`, { method: "POST", json: { watched } });
      clear();
      await reload();
    })();


  const resetFilters = () => {
    setFilter("all");
    setGenre("");
    setSort("title");
  };
  const title = kind === "movie" ? "Movies" : "TV shows";
  const noun = kind === "movie" ? "movie" : "show";
  const total = data?.length ?? 0;

  return (
    <div className="flex h-full flex-col">
      <header className="gutter border-b border-border-light pb-3 pt-5">
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
            onReset={resetFilters}
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
                { id: "list", label: "My list" },
                { id: "unmatched", label: "Unmatched" },
              ]}
            />
            {genres.length > 0 && (
              <Choices label="Genre" value={genre} onChange={setGenre} options={[{ id: "", label: "All genres" }, ...genres.map((g) => ({ id: g, label: g }))]} />
            )}
          </FilterMenu>
        </div>
        {selecting && (
          <div className="mt-2 flex flex-wrap items-center gap-2 text-sm">
            <span className="font-medium text-content">{chosen.length.toLocaleString()} selected</span>
            <MenuButton
              label="Actions for the selected titles"
              className="btn-ghost"
              icon={
                <>
                  Actions <ChevronDown size={14} />
                </>
              }
              items={[
                { id: "play", label: "Play all", detail: "One after another, in this order", icon: <Play size={14} />, onSelect: () => void playAll() },
                allListed
                  ? { id: "unlist", label: "Remove from My list", icon: <BookmarkX size={14} />, onSelect: () => void setListed(false) }
                  : { id: "list", label: "Add to My list", icon: <Bookmark size={14} />, onSelect: () => void setListed(true) },
                { id: "watched", label: "Mark watched", icon: <Eye size={14} />, onSelect: () => void setWatched(true) },
                { id: "unwatched", label: "Mark unwatched", icon: <EyeOff size={14} />, onSelect: () => void setWatched(false) },
                { id: "refresh", label: "Refresh metadata", detail: "Fetches details and artwork again; a fixed match stays", icon: <RefreshCw size={14} />, onSelect: () => void refreshMeta() },
                ...(comskip ? [{ id: "commercials", label: "Find commercials", detail: "In recordings not yet checked", icon: <Scissors size={14} />, onSelect: () => void findCommercials() }] : []),
                ...(chosen.length > 1
                  ? [{ id: "merge", label: "Merge…", detail: kind === "movie" ? "Into one film: duplicates or bonus material" : "Into one show", icon: <Combine size={14} />, onSelect: () => setMerging(true) }]
                  : []),
                ...(chosen.length === 1
                  ? [
                      {
                        id: "edit",
                        label: "Edit…",
                        detail: "Details, match, artwork and files, on its page",
                        icon: <Settings2 size={14} />,
                        onSelect: () => go(`/item/${chosen[0].id}?edit=1`),
                      },
                    ]
                  : []),
                { id: "delete", label: "Delete…", detail: "Removes the files from disk", icon: <Trash2 size={14} />, danger: true, onSelect: () => setDeleting(true) },
              ]}
            />
            {chosen.length < items.length && (
              <button type="button" className="btn-quiet" onClick={() => setPicked(new Set(items.map((i) => i.id)))}>
                Select all {items.length.toLocaleString()}
              </button>
            )}
            <button type="button" className="btn-quiet" onClick={clear}>
              Clear
            </button>
          </div>
        )}
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
        <Loading fill />
      ) : total === 0 ? (
        <EmptyState icon={kind === "movie" ? <Clapperboard size={36} strokeWidth={1.5} /> : <Tv size={36} strokeWidth={1.5} />} title={`No ${noun}s yet`}>
          {admin ? (
            <>
              Add a {kind === "movie" ? "Movies" : "TV shows"} library on the <Link to="/libraries" className="text-accent hover:underline">Libraries</Link> page,
              or wait for the current scan to finish.
            </>
          ) : (
            "Nothing has been scanned in yet. Ask an admin to add a library."
          )}
        </EmptyState>
      ) : items.length === 0 ? (
        <EmptyState
          icon={<SearchX size={36} strokeWidth={1.5} />}
          title="Nothing matches those filters"
          action={
            <button className="btn-ghost" onClick={resetFilters}>
              Reset filters
            </button>
          }
        />
      ) : (
        <PosterGrid items={items} memoryKey={kind} selection={admin ? { ids: picked, onClick: pick } : undefined} />
      )}
      {merging && (
        <MergeTitles
          items={chosen}
          onClose={() => setMerging(false)}
          onMerged={() => {
            clear();
            void reload();
          }}
        />
      )}
      {deleting && (
        <DeleteSelected
          items={chosen}
          onClose={() => setDeleting(false)}
          onDeleted={() => {
            clear();
            void reload();
          }}
        />
      )}
    </div>
  );
}
