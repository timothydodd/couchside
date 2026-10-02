import { ArrowLeft } from "lucide-react";
import { PersonPhoto } from "../components/Credits";
import PosterCard from "../components/PosterCard";
import { EmptyState, ErrorNote, Spinner } from "../components/ui";
import { useApi } from "../lib/api";
import type { PersonDetail } from "../lib/types";
import { useRouter } from "../stores/router";

/** Everything in the library a cast or crew member is in, newest first. */
export default function PersonPage({ id }: { id: number }) {
  const { data, error, loading } = useApi<PersonDetail>(`/api/people/${id}`);
  const back = useRouter((s) => s.back);

  if (loading && !data)
    return (
      <div className="flex h-full items-center justify-center">
        <Spinner size={22} />
      </div>
    );
  if (!data) return <EmptyState title="Person not found">{error}</EmptyState>;

  const { person, items } = data;
  const movies = items.filter((i) => i.kind === "movie");
  const shows = items.filter((i) => i.kind === "series");

  return (
    <div className="pb-8">
      <div className="px-6 pt-4">
        <button className="btn-quiet" onClick={() => back("/")}>
          <ArrowLeft size={15} /> Back
        </button>
      </div>
      <header className="flex items-end gap-5 px-6 pb-2 pt-4">
        <PersonPhoto person={person} className="w-32 shrink-0" />
        <div className="min-w-0 pb-1">
          <h1 className="text-3xl font-bold leading-tight text-content">{person.name}</h1>
          <p className="mt-1 text-sm text-content-muted">
            {items.length} {items.length === 1 ? "title" : "titles"} in your library
          </p>
        </div>
      </header>
      {error && (
        <div className="px-6 pt-4">
          <ErrorNote>{error}</ErrorNote>
        </div>
      )}
      {[
        { title: "Movies", list: movies },
        { title: "TV shows", list: shows },
      ].map(
        (g) =>
          g.list.length > 0 && (
            <section key={g.title} className="px-6 py-3">
              <h2 className="row-title mb-3">{g.title}</h2>
              <div className="grid grid-cols-[repeat(auto-fill,minmax(10rem,1fr))] gap-5">
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
    </div>
  );
}
