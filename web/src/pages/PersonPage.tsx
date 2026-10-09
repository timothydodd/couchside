import { PersonPhoto } from "../components/Credits";
import FadeImg from "../components/FadeImg";
import EpisodeCard from "../components/EpisodeCard";
import PosterCard from "../components/PosterCard";
import { BackButton, EmptyState, ErrorNote, Loading, Section } from "../components/ui";
import { personPhotoUrl, useApi } from "../lib/api";
import { usePhone } from "../lib/media";
import type { PersonDetail } from "../lib/types";
import { useTitle } from "../lib/title";

/** Everything in the library a cast or crew member is in, newest first. */
export default function PersonPage({ id }: { id: number }) {
  const { data, error, loading, reload } = useApi<PersonDetail>(`/api/people/${id}`);
  useTitle(data?.person.name);
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
      {/* A hero like a title's: their photo blown up and blurred, as a title with no backdrop gets its poster. */}
      <section className="relative h-48 overflow-hidden sm:h-56">
        {person.hasPhoto ? (
          <FadeImg src={personPhotoUrl(person.id)} alt="" className="absolute inset-0 h-full w-full scale-110 object-cover object-top blur-2xl" style={{ "--img-opacity": 0.5 } as React.CSSProperties} />
        ) : (
          <div className="poster-placeholder absolute inset-0" />
        )}
        <div className="hero-fade absolute inset-0" />
        <BackButton fallback="/" overlay />
      </section>
      <header className="relative -mt-20 flex items-end gap-4 gutter pb-2 sm:-mt-24 sm:gap-5">
        <PersonPhoto person={person} className="w-24 shrink-0 shadow-[var(--shadow-poster)] sm:w-32" />
        <div className="min-w-0 pb-1">
          <h1 className="text-2xl font-bold leading-tight text-content sm:text-3xl">{person.name}</h1>
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
            <Section key={g.title} title={g.title}>
              <div className="poster-wrap">
                {g.list.map((it) => (
                  <div key={it.id}>
                    <PosterCard item={it} />
                    {it.roles.length > 0 && <div className="poster-meta -mt-0.5 truncate">{it.roles.join(", ")}</div>}
                  </div>
                ))}
              </div>
            </Section>
          ),
      )}
      {episodes.length > 0 && (
        <Section title="Episodes">
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
        </Section>
      )}
    </div>
  );
}
