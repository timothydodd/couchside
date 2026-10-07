import { useRef } from "react";
import { FolderPlus, Info, Play, ScanSearch, Sofa } from "lucide-react";
import Link from "../components/Link";
import { ContinueCard, PosterRow, Row } from "../components/Rows";
import { EmptyState, ErrorNote, StatTile, Loading } from "../components/ui";
import { api, backdropUrl, useApi } from "../lib/api";
import { fmtClock } from "../lib/format";
import { playTarget } from "../lib/items";
import { attempt } from "../lib/notices";
import type { Home, ItemDetail, ItemSummary } from "../lib/types";
import { useIsAdmin } from "../stores/auth";
import { useRouter } from "../stores/router";
import { useStatus } from "../stores/status";

export default function HomePage() {
  const { data, error, loading, reload } = useApi<Home>("/api/home", { pollMs: 15000 });
  const remove = attempt("Couldn't remove it", async (itemId: number) => {
    await api(`/api/home/continue/${itemId}`, { method: "DELETE" });
    await reload();
  });
  const status = useStatus((s) => s.status);
  const go = useRouter((s) => s.go);
  const admin = useIsAdmin();
  const counts = status?.counts;
  // Picked once, so the 15s poll doesn't swap the hero while it's being read; re-picked only if it leaves the rows.
  const hero = useRef<ItemSummary | undefined>(undefined);
  const recent = [...(data?.recentMovies ?? []), ...(data?.recentSeries ?? [])];
  if (data && (!hero.current || !recent.some((i) => i.id === hero.current!.id))) hero.current = pickFeatured(recent);
  const featured = hero.current;
  const scanning = !!status && status.jobs.running + status.jobs.queued > 0;

  if (loading && !data) {
    return (
      <Loading fill />
    );
  }
  if (counts && counts.libraries === 0) {
    return (
      <EmptyState icon={<Sofa size={36} strokeWidth={1.5} />} title="The couch is empty">
        Point Couchside at a folder of movies or TV shows and it will scan them, fetch posters and plots, and fill this page.
        {admin ? (
          <div className="mt-4">
            <Link to="/libraries" className="btn-primary">
              <FolderPlus size={15} /> Add a library
            </Link>
          </div>
        ) : (
          " An admin can add one."
        )}
      </EmptyState>
    );
  }

  if (counts && counts.movies + counts.series === 0) {
    return (
      <EmptyState
        icon={scanning ? <ScanSearch size={36} strokeWidth={1.5} className="animate-pulse" /> : <Sofa size={36} strokeWidth={1.5} />}
        title={scanning ? "Scanning your libraries…" : "No titles yet"}
        action={
          admin && !scanning ? (
            <Link to="/libraries" className="btn-primary">
              <ScanSearch size={15} /> Scan now
            </Link>
          ) : undefined
        }
      >
        {scanning
          ? "Movies and shows appear here as they're found; posters and plots follow."
          : admin
            ? "The libraries' folders had no video files Couchside recognises. Check the folders on the Libraries page, then scan."
            : "Nothing has been scanned in yet. Ask an admin to check the libraries."}
      </EmptyState>
    );
  }

  return (
    <div className="pb-8">
      {featured ? <Hero item={featured} /> : <div className="h-4" />}
      {error && (
        <div className="gutter pt-4">
          <ErrorNote>{error}</ErrorNote>
        </div>
      )}
      {counts && (
        <div className="hidden grid-cols-2 gap-3 gutter py-4 sm:grid md:grid-cols-4">
          <StatTile label="Movies" value={counts.movies.toLocaleString()} onClick={() => go("/movies")} />
          <StatTile label="TV shows" value={counts.series.toLocaleString()} onClick={() => go("/tv")} />
          <StatTile label="Episodes" value={counts.episodes.toLocaleString()} />
          <StatTile
            label="Unmatched"
            value={counts.unmatched.toLocaleString()}
            sub={
              !admin ? undefined : status?.providers.length ? (
                "Fix from the title's page"
              ) : (
                <Link to="/settings/metadata" className="hover:text-accent">
                  Add a provider to match
                </Link>
              )
            }
            tone={counts.unmatched ? "warning" : undefined}
          />
        </div>
      )}
      {!!data?.continueWatching.length && (
        <Row title="Continue watching">
          {data.continueWatching.map((p) => (
            <ContinueCard key={p.fileId} p={p} onRemove={() => void remove(p.itemId)} />
          ))}
        </Row>
      )}
      <PosterRow title="My list" items={data?.watchlist ?? []} />
      <PosterRow
        title="Recently added movies"
        items={data?.recentMovies ?? []}
        action={<Link to="/movies" className="text-xs text-content-muted hover:text-accent">See all</Link>}
      />
      <PosterRow
        title="Recently updated shows"
        items={data?.recentSeries ?? []}
        action={<Link to="/tv" className="text-xs text-content-muted hover:text-accent">See all</Link>}
      />
    </div>
  );
}

/** Newest title with a backdrop, preferring matched ones (they have a plot). */
function pickFeatured(items: ItemSummary[]): ItemSummary | undefined {
  const withArt = items.filter((i) => i.hasBackdrop).sort((a, b) => b.lastAddedAt - a.lastAddedAt);
  return withArt.find((i) => i.matchStatus === "matched") ?? withArt[0];
}

/** The newest title, big: its plot, and Play goes straight to the file (resuming if it was started). */
function Hero({ item }: { item: ItemSummary }) {
  const { data } = useApi<ItemDetail>(`/api/items/${item.id}`);
  const plot = data?.item.plot;
  const play = data ? playTarget(data) : null;
  return (
    <section className="relative h-[52vh] min-h-80 max-h-[520px] overflow-hidden md:h-[46vh] md:min-h-72">
      <img src={backdropUrl(item)} alt="" className="hero-art" />
      <div className="hero-fade absolute inset-0" />
      <div className="absolute inset-x-0 bottom-0 max-w-2xl gutter pb-6">
        <div className="text-[11px] font-semibold uppercase tracking-widest brand-text">Just added</div>
        <h1 className="mt-1 text-2xl font-bold leading-tight text-content sm:text-3xl md:text-4xl">
          <Link to={`/item/${item.id}`} className="title-link">
            {item.title}
          </Link>
        </h1>
        <div className="mt-1 text-sm text-content-secondary">
          {[item.year, item.genres.slice(0, 3).join(", ")].filter(Boolean).join(" · ")}
        </div>
        {plot && <p className="mt-2 line-clamp-2 text-sm text-content-secondary">{plot}</p>}
        <div className="mt-4 flex gap-2">
          <Link to={play ? `/play/${play.fileId}` : `/item/${item.id}`} className="btn-primary">
            <Play size={15} className="fill-current" /> {play && play.resumeAt > 30 ? `Resume ${fmtClock(play.resumeAt)}` : "Play"}
          </Link>
          <Link to={`/item/${item.id}`} className="btn-ghost !bg-surface/60 backdrop-blur">
            <Info size={15} /> Details
          </Link>
        </div>
      </div>
    </section>
  );
}
