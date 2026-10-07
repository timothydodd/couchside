import { ArrowLeft } from "lucide-react";
import { PersonPhoto } from "../components/Credits";
import EpisodeCard from "../components/EpisodeCard";
import PosterCard from "../components/PosterCard";
import { EmptyState, ErrorNote, Loading } from "../components/ui";
import { useApi } from "../lib/api";
import { usePhone } from "../lib/media";
import type { PersonDetail } from "../lib/types";
import { useRouter } from "../stores/router";

/** Everything in the library a cast or crew member is in, newest first. */
export default function PersonPage({ id }: { id: number }) {
  const { data, error, loading, reload } = useApi<PersonDetail>(`/api/people/${id}`);
  const back = useRouter((s) => s.back);
  const phone = usePhone();

  if (loading && !data)
    return (
      <Loading fill />
    );
  if (!data) {
    // Only a 404 means it isn't there; anything else is a failed request.
    const missing = !error || /not found/i.test(error);
    return (
      <EmptyState title={missing ? "Person not found" : "Couldn't load this person"}>
        {!missing && error}
        {!missing && (
          <div className="mt-3">
            <button className="btn-ghost" onClick={() => void reload()}>
              Try again
            </button>
          </div>
        )}
      </EmptyState>
    );
  }

  const { person, items, episodes } = data;
  const movies = items.filter((i) => i.kind === "movie");
  const shows = items.filter((i) => i.kind === "series");
  const count = items.length + episodes.length;

  return (
    <div className="pb-8">
      <div className="gutter pt-4">
        <button className="btn-quiet" onClick={() => back("/")}>
          <ArrowLeft size={15} /> Back
        </button>
      </div>
      <header className="flex items-end gap-5 gutter pb-2 pt-4">
        <PersonPhoto person={person} className="w-32 shrink-0" />
        <div className="min-w-0 pb-1">
          <h1 className="text-3xl font-bold leading-tight text-content">{person.name}</h1>
          <p className="mt-1 text-sm text-content-muted">
            {count} {count === 1 ? "title" : "titles"} in your library
          </p>
        </div>
      </header>
      {error && (
        <div className="gutter pt-4">
          <ErrorNote>{error}</ErrorNote>
        </div>
      )}
      {[
        { title: "Movies", list: movies },
        { title: "TV shows", list: shows },
      ].map(
        (g) =>
          g.list.length > 0 && (
            <section key={g.title} className="gutter py-3">
              <h2 className="row-title mb-3">{g.title}</h2>
              <div className="poster-wrap">
                {g.list.map((it) => (
                  <div key={it.id}>
                    <PosterCard item={it} />
                    {it.roles.length > 0 && <div className="poster-meta -mt-0.5 truncate">{it.roles.join(", ")}</div>}
                  </div>
                ))}
              </div>
            </section>
          ),
      )}
      {episodes.length > 0 && (
        <section className="gutter py-3">
          <h2 className="row-title mb-3">Episodes</h2>
          <ol className={phone ? "flex flex-col" : "grid grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-x-4 gap-y-5"}>
            {episodes.map((e) => (
              <li key={e.id}>
                <EpisodeCard
                  file={e}
                  layout={phone ? "row" : "grid"}
                  eyebrow={`S${e.season} E${e.episode}`}
                  title={e.title || `Episode ${e.episode}`}
                  to={`/episode/${e.id}`}
                  playLabel={`Play ${e.seriesTitle} S${e.season} E${e.episode}`}
                  meta={
                    <>
                      <span className="text-content-secondary">{e.seriesTitle}</span>
                      {e.roles.length > 0 && <span>{e.roles.join(", ")}</span>}
                    </>
                  }
                />
              </li>
            ))}
          </ol>
        </section>
      )}
    </div>
  );
}
