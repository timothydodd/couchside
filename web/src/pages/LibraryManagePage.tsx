import { memo, useMemo, useState } from "react";
import { ArrowDown, ArrowLeft, ArrowUp, Clapperboard, Copy, Cpu, ImageUp, Pencil, RefreshCw, ScanSearch, Trash2, Tv } from "lucide-react";
import Link from "../components/Link";
import LibraryForm from "../components/LibraryForm";
import { PosterArt } from "../components/PosterCard";
import { EmptyState, ErrorNote, PageHeader, SearchInput, Spinner } from "../components/ui";
import { api, useApi } from "../lib/api";
import { fmtAgo, fmtBytes } from "../lib/format";
import { codecLabel, extraFiles, qualityLabel, qualityTier, qualityTone } from "../lib/quality";
import type { Library, ManageRow } from "../lib/types";
import { useRouter } from "../stores/router";
import { useStatus } from "../stores/status";
import { attempt, notify } from "../lib/notices";
import { confirmDialog } from "../lib/ask";

type Filter = "all" | "duplicates" | "unmatched" | "low";
type SortKey = "title" | "quality" | "files" | "size" | "added";

const optimizeAll = attempt("Couldn't queue the encodes", async (l: Library) => {
  if (
    !(await confirmDialog({
      title: `Optimize everything in "${l.name}"?`,
      body: "Encodes browser-friendly copies of every file that can't play directly. This runs in the background, one file at a time, and the copies take disk space in the cache volume (roughly 1-4 GB per movie at 1080p).",
      action: "Optimize",
    }))
  )
    return;
  const r = await api<{ queued: number }>(`/api/libraries/${l.id}/optimize`, { method: "POST" });
  notify(r.queued ? `Queued ${r.queued} encode${r.queued === 1 ? "" : "s"}; progress is on the Activity page.` : "Nothing to do: everything already plays directly or has an optimized copy.", "info");
  void useStatus.getState().refresh();
});

const isDuplicate = (r: ManageRow) => extraFiles(r) > 0 || r.sameImdb > 0;
const isLow = (r: ManageRow) => qualityTier(r.maxHeight) <= 1;

const SORTS: Record<SortKey, (a: ManageRow, b: ManageRow) => number> = {
  title: (a, b) => a.title.localeCompare(b.title, undefined, { sensitivity: "base", ignorePunctuation: true }),
  quality: (a, b) => a.maxHeight - b.maxHeight || a.minHeight - b.minHeight || a.size - b.size,
  files: (a, b) => a.fileCount - b.fileCount,
  size: (a, b) => a.size - b.size,
  added: (a, b) => a.addedAt - b.addedAt,
};

const scan = attempt("Couldn't start the scan", async (id: number) => {
  await api(`/api/libraries/${id}/scan`, { method: "POST" });
  notify("Scanning; progress is on the Activity page.", "info");
  void useStatus.getState().refresh();
});
const rematch = attempt("Couldn't re-match the library", async (l: Library) => {
  if (!(await confirmDialog({ title: `Look up every title in "${l.name}" again?`, body: "This refreshes details, posters, backdrops and cast from the metadata providers. Matches you fixed by hand stay as they are.", action: "Re-match" }))) return;
  const r = await api<{ queued: number }>(`/api/libraries/${l.id}/rematch`, { method: "POST" });
  notify(`Queued ${r.queued} lookup${r.queued === 1 ? "" : "s"}; progress is on the Activity page.`, "info");
  void useStatus.getState().refresh();
});

/**
 * A library's settings: its name, folder and options, scans and bulk jobs,
 * and every title as a sortable table (quality, duplicates, unmatched).
 * A title's row opens its page with the Edit panel.
 */
export default function LibraryManagePage({ id }: { id: number }) {
  const go = useRouter((s) => s.go);
  const [editing, setEditing] = useState(false);
  const [anyPending, setAnyPending] = useState(false);
  const { data, error, reload } = useApi<{ library: Library; items: ManageRow[] }>(`/api/libraries/${id}/manage`, {
    pollMs: anyPending ? 3000 : undefined,
  });
  const [q, setQ] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const [sort, setSort] = useState<{ key: SortKey; desc: boolean }>({ key: "title", desc: false });

  const items = useMemo(() => data?.items ?? [], [data]);
  const pending = items.some((r) => r.matchStatus === "pending");
  if (pending !== anyPending) setAnyPending(pending);

  const counts = useMemo(
    () => ({
      all: items.length,
      duplicates: items.filter(isDuplicate).length,
      unmatched: items.filter((r) => r.matchStatus === "unmatched").length,
      low: items.filter(isLow).length,
    }),
    [items],
  );
  const rows = useMemo(() => {
    const n = q.trim().toLowerCase();
    const out = items.filter(
      (r) =>
        (filter === "all" || (filter === "duplicates" && isDuplicate(r)) || (filter === "unmatched" && r.matchStatus === "unmatched") || (filter === "low" && isLow(r))) &&
        (!n || r.title.toLowerCase().includes(n) || r.parsedTitle.toLowerCase().includes(n)),
    );
    const cmp = SORTS[sort.key];
    out.sort((a, b) => (sort.desc ? -cmp(a, b) : cmp(a, b)) || SORTS.title(a, b));
    return out;
  }, [items, q, filter, sort]);

  const lib = data?.library;
  const totalSize = items.reduce((s, r) => s + r.size, 0);
  const movies = lib?.kind === "movies";
  const remove = attempt("Couldn't remove the library", async (l: Library) => {
    if (!(await confirmDialog({ title: `Remove "${l.name}" from Couchside?`, body: "Your files are not touched; only the library entry, watch history and artwork go.", action: "Remove", danger: true }))) return;
    await api(`/api/libraries/${l.id}`, { method: "DELETE" });
    void useStatus.getState().refresh();
    go("/libraries", { replace: true });
  });

  const header = (key: SortKey, label: string, className = "") => (
    <th className={className} aria-sort={sort.key === key ? (sort.desc ? "descending" : "ascending") : undefined}>
      <button
        className="inline-flex items-center gap-1 hover:text-content"
        onClick={() => setSort((s) => ({ key, desc: s.key === key ? !s.desc : key !== "title" }))}
      >
        {label}
        {sort.key === key && (sort.desc ? <ArrowDown size={12} /> : <ArrowUp size={12} />)}
      </button>
    </th>
  );

  return (
    <div>
      <PageHeader
        title={lib ? lib.name : "Library"}
        subtitle={lib ? `${items.length.toLocaleString()} ${movies ? (items.length === 1 ? "movie" : "movies") : items.length === 1 ? "show" : "shows"} · ${lib.fileCount.toLocaleString()} files · ${fmtBytes(totalSize)} · scanned ${fmtAgo(lib.lastScanAt)}` : undefined}
      >
        <Link to="/libraries" className="btn-quiet">
          <ArrowLeft size={15} /> Libraries
        </Link>
      </PageHeader>
      <div className="flex flex-col gap-3 gutter py-4">
        {error && <ErrorNote>{error}</ErrorNote>}
        {lib &&
          (editing ? (
            <LibraryForm
              library={lib}
              onDone={() => {
                setEditing(false);
                void reload();
                void useStatus.getState().refresh();
              }}
              onCancel={() => setEditing(false)}
            />
          ) : (
            <div className="card flex flex-wrap items-center gap-4 px-4 py-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-md bg-muted text-accent">{movies ? <Clapperboard size={18} /> : <Tv size={18} />}</div>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-sm font-semibold text-content">{lib.name}</span>
                  <span className="badge tint-muted">{movies ? "Movies" : "TV shows"}</span>
                  {lib.trickplay && <span className="badge tint-muted">Preview thumbnails</span>}
                  {lib.intros && <span className="badge tint-muted">Finds intros</span>}
                </div>
                <div className="mono mt-0.5 truncate text-content-muted">{lib.path}</div>
              </div>
              <div className="flex flex-wrap gap-2">
                <button className="btn-ghost" onClick={() => setEditing(true)}>
                  <Pencil size={15} /> Edit
                </button>
                <button className="btn-ghost" onClick={() => void scan(lib.id)} title="Look for new, changed and removed files">
                  <ScanSearch size={15} /> Scan
                </button>
                <button className="btn-ghost" onClick={() => void rematch(lib)} title="Look up details, artwork and cast again for every title">
                  <RefreshCw size={15} /> Re-match
                </button>
                <button className="btn-ghost" onClick={() => void optimizeAll(lib)} title="Encode browser-friendly copies of every file that needs transcoding">
                  <Cpu size={15} /> Optimize all
                </button>
                <button className="btn-ghost hover:!border-critical hover:!text-critical" onClick={() => void remove(lib)} title="Files on disk stay; its watch history and artwork go">
                  <Trash2 size={15} /> Remove
                </button>
              </div>
            </div>
          ))}
        <h2 className="row-title mt-3">Titles</h2>
        <div className="flex flex-wrap items-center gap-2">
          <SearchInput value={q} onChange={setQ} placeholder={movies ? "Find a movie…" : "Find a show…"} className="w-full sm:w-64" />
          <div className="inline-flex rounded-md border border-border p-0.5" role="radiogroup" aria-label="Show">
            {(
              [
                ["all", "All"],
                ["duplicates", "Duplicates"],
                ["unmatched", "Unmatched"],
                ["low", "SD / unknown"],
              ] as [Filter, string][]
            ).map(([f, label]) => (
              <button
                key={f}
                role="radio"
                aria-checked={filter === f}
                onClick={() => setFilter(f)}
                className={`rounded px-3 py-1 text-sm transition-colors ${filter === f ? "bg-accent text-on-accent" : "text-content-secondary hover:text-content"}`}
              >
                {label} <span className="tabular-nums opacity-70">{counts[f]}</span>
              </button>
            ))}
          </div>
        </div>

        {!data ? (
          <div className="flex justify-center py-16">
            <Spinner size={22} />
          </div>
        ) : rows.length === 0 ? (
          <div className="card">
            <EmptyState title={filter === "duplicates" ? "No duplicates" : filter === "unmatched" ? "Everything is matched" : "Nothing here"}>
              {filter !== "all" ? "Nothing in this library fits that filter." : "Scan the library to fill it."}
            </EmptyState>
          </div>
        ) : (
          <div className="card overflow-x-auto">
            <table className="table">
              <thead>
                <tr>
                  <th className="w-12" />
                  {header("title", movies ? "Movie" : "Show")}
                  {header("quality", "Quality")}
                  {header("files", "Files")}
                  {header("size", "Size", "text-right")}
                  {header("added", "Added")}
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => (
                  <Row key={r.id} r={r} />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

// Memoised on the row's content: the list is fetched again every few
// seconds while anything is matching, and only rows that changed should
// render again.
const Row = memo(RowView, (a, b) => JSON.stringify(a.r) === JSON.stringify(b.r));

/** One title; its name opens its page with the Edit panel. */
function RowView({ r }: { r: ManageRow }) {
  const extra = extraFiles(r);
  const range = r.kind === "series" && r.minHeight && qualityTier(r.minHeight) !== qualityTier(r.maxHeight);
  return (
    <tr>
      <td className="!py-1">
        <Link to={`/item/${r.id}`} className="poster relative block h-12 w-8 overflow-hidden rounded" aria-label={r.title}>
          <PosterArt item={r} bare />
        </Link>
      </td>
      <td className="max-w-md">
        <div className="flex min-w-0 items-center gap-2">
          <Link to={`/item/${r.id}?edit=1`} className="title-link truncate font-medium text-content">
            {r.title}
          </Link>
          {r.year && <span className="text-content-muted">{r.year}</span>}
          {r.matchStatus === "unmatched" && <span className="badge tint-warning">Unmatched</span>}
          {r.matchStatus === "pending" && <span className="badge tint-info">Matching…</span>}
          {r.customPoster && <ImageUp size={13} className="shrink-0 text-content-muted" aria-label="Custom poster" />}
        </div>
        {r.parsedTitle.toLowerCase() !== r.title.toLowerCase() && <div className="truncate text-xs text-content-muted">From files: {r.parsedTitle}</div>}
      </td>
      <td>
        <span className={`badge tint-${qualityTone(r.maxHeight)}`}>
          {range ? `${qualityLabel(r.minHeight)}–${qualityLabel(r.maxHeight)}` : qualityLabel(r.maxHeight)}
        </span>
        {r.videoCodec && <span className="ml-1.5 text-xs text-content-muted">{codecLabel(r.videoCodec)}</span>}
      </td>
      <td className="tabular-nums">
        {r.kind === "series" ? `${r.episodeCount} ep${r.episodeCount === 1 ? "" : "s"}` : r.fileCount}
        {extra > 0 && (
          <span className="badge tint-warning ml-1.5" title="More than one file for the same movie or episode">
            <Copy size={11} /> +{extra} {extra === 1 ? "copy" : "copies"}
          </span>
        )}
        {r.parts > 0 && <span className="badge tint-muted ml-1.5">{r.parts} parts</span>}
        {r.extras > 0 && <span className="badge tint-muted ml-1.5">{r.extras} extra{r.extras === 1 ? "" : "s"}</span>}
        {r.sameImdb > 0 && (
          <span className="badge tint-info ml-1.5" title="Another entry in this library is matched to the same title">
            Same title ×{r.sameImdb + 1}
          </span>
        )}
      </td>
      <td className="text-right tabular-nums">{fmtBytes(r.size)}</td>
      <td className="text-content-muted">{fmtAgo(r.addedAt)}</td>
    </tr>
  );
}
