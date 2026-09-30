import { FolderPlus, Info, Play, Sofa } from "lucide-react";
import Link from "../components/Link";
import { ContinueCard, PosterRow, Row } from "../components/Rows";
import { EmptyState, ErrorNote, Spinner, StatTile } from "../components/ui";
import { backdropUrl, useApi } from "../lib/api";
import type { Home, ItemSummary } from "../lib/types";
import { useRouter } from "../stores/router";
import { useStatus } from "../stores/status";

export default function HomePage() {
  const { data, error, loading } = useApi<Home>("/api/home", { pollMs: 15000 });
  const status = useStatus((s) => s.status);
  const go = useRouter((s) => s.go);
  const counts = status?.counts;

  if (loading && !data) {
    return (
      <div className="flex h-full items-center justify-center">
        <Spinner size={22} />
      </div>
    );
  }
  if (counts && counts.libraries === 0) {
    return (
      <EmptyState icon={<Sofa size={40} strokeWidth={1.5} />} title="The couch is empty">
        Point Couchside at a folder of movies or TV shows and it will scan them, fetch posters and plots, and fill this page.
        <div className="mt-4">
          <Link to="/libraries" className="btn-primary">
            <FolderPlus size={15} /> Add a library
          </Link>
        </div>
      </EmptyState>
    );
  }

  const featured = pickFeatured([...(data?.recentMovies ?? []), ...(data?.recentSeries ?? [])]);

  return (
    <div className="pb-8">
      {featured ? <Hero item={featured} /> : <div className="h-4" />}
      {error && (
        <div className="px-6 pt-4">
          <ErrorNote>{error}</ErrorNote>
        </div>
      )}
      {counts && (
        <div className="grid grid-cols-2 gap-3 px-6 py-4 md:grid-cols-4">
          <StatTile label="Movies" value={counts.movies.toLocaleString()} onClick={() => go("/movies")} />
          <StatTile label="TV shows" value={counts.series.toLocaleString()} onClick={() => go("/tv")} />
          <StatTile label="Episodes" value={counts.episodes.toLocaleString()} />
          <StatTile
            label="Unmatched"
            value={counts.unmatched.toLocaleString()}
            sub={status?.providers.length ? "Fix from the title's page" : "Set OMDB_API_KEY to match"}
            tone={counts.unmatched ? "warning" : undefined}
          />
        </div>
      )}
      {!!data?.continueWatching.length && (
        <Row title="Continue watching">
          {data.continueWatching.map((p) => (
            <ContinueCard key={p.fileId} p={p} />
          ))}
        </Row>
      )}
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

function Hero({ item }: { item: ItemSummary }) {
  const { data } = useApi<{ item: { plot: string } }>(`/api/items/${item.id}`);
  const plot = data?.item.plot;
  return (
    <section className="relative h-[46vh] min-h-72 max-h-[520px] overflow-hidden">
      <img src={backdropUrl(item)} alt="" className="absolute inset-0 h-full w-full object-cover" />
      <div className="hero-fade absolute inset-0" />
      <div className="absolute inset-x-0 bottom-0 max-w-2xl px-6 pb-6">
        <div className="text-[11px] font-semibold uppercase tracking-widest text-brand-pink">Just added</div>
        <h1 className="mt-1 text-3xl font-bold leading-tight text-content md:text-4xl">{item.title}</h1>
        <div className="mt-1 text-sm text-content-secondary">
          {[item.year, item.genres.slice(0, 3).join(", ")].filter(Boolean).join(" · ")}
        </div>
        {plot && <p className="mt-2 line-clamp-2 text-sm text-content-secondary">{plot}</p>}
        <div className="mt-4 flex gap-2">
          <Link to={`/item/${item.id}`} className="btn-primary">
            <Play size={15} className="fill-current" /> Watch
          </Link>
          <Link to={`/item/${item.id}`} className="btn-ghost !bg-surface/60 backdrop-blur">
            <Info size={15} /> Details
          </Link>
        </div>
      </div>
    </section>
  );
}
