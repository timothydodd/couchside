import { useState } from "react";
import { ChevronLeft, ChevronRight, Eye, EyeOff, Pencil, Play, Star } from "lucide-react";
import { CastRow, CrewLine } from "../components/Credits";
import FilesCard from "../components/item/FilesCard";
import { EpisodeActions } from "../components/item/AdminMenus";
import ItemPanel from "../components/manage/ItemPanel";
import Link from "../components/Link";
import FadeImg from "../components/FadeImg";
import { BackButton, EmptyState, ErrorNote, Loading } from "../components/ui";
import { api, backdropUrl, stillUrl, useApi } from "../lib/api";
import { fmtAirDate, fmtClock, fmtResolution, fmtRuntime } from "../lib/format";
import { attempt } from "../lib/notices";
import RangeChip from "../components/RangeChip";
import { PROBLEM_TEXT, type EpisodeDetail, type EpisodeRef } from "../lib/types";
import { useIsAdmin } from "../stores/auth";

/** One episode: its still, synopsis, guest stars and crew, its copies, and the episodes either side. */
export default function EpisodePage({ id }: { id: number }) {
  const { data, error, loading, reload } = useApi<EpisodeDetail>(`/api/episodes/${id}`);
  const admin = useIsAdmin();
  const [editing, setEditing] = useState(false);

  if (loading && !data) {
    return (
      <Loading fill />
    );
  }
  if (!data) {
    const missing = !error || /not found/i.test(error);
    return (
      <EmptyState title={missing ? "Episode not found" : "Couldn't load this episode"}>
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

  const { episode: e, series, files, prev, next } = data;
  const best = files[0];
  const number = e.airDate ? fmtAirDate(e.airDate) : `S${e.season} · E${e.episode}`;
  const showPage = `/item/${series.id}?season=${e.season}`;
  const runtime = e.runtimeMin ? fmtRuntime(e.runtimeMin * 60) : fmtRuntime(e.durationSec);
  const resumeAt = !e.watched && e.positionSec > 30 ? e.positionSec : 0;

  const setWatched = attempt("Couldn't change watched", async (watched: boolean) => {
    await api(`/api/files/${e.fileId}/watched`, { method: "POST", json: { watched } });
    await reload();
  });

  return (
    <div className="pb-10">
      {/* The still as the hero, or the show's backdrop when there's none. */}
      <section className="relative h-[40vh] min-h-64 max-h-[460px] overflow-hidden">
        {e.hasStill ? (
          <FadeImg src={stillUrl(e.fileId)} alt="" className="hero-art" />
        ) : series.hasBackdrop ? (
          <FadeImg src={backdropUrl(series)} alt="" className="hero-art" />
        ) : (
          <div className="poster-placeholder absolute inset-0" />
        )}
        <div className="hero-fade absolute inset-0" />
        <BackButton fallback={showPage} overlay />
      </section>

      <div className="relative -mt-24 gutter sm:-mt-32">
        <Link to={showPage} className="title-link text-sm font-semibold text-content-secondary">
          {series.title}
        </Link>
        <h1 className="mt-1 text-2xl font-bold leading-tight text-content sm:text-3xl md:text-4xl">
          <span className="text-content-muted">{number}</span>
          {e.title && <span> · {e.title}</span>}
        </h1>
        <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-content-secondary">
          {e.released && <span>{e.released}</span>}
          {runtime && <span>{runtime}</span>}
          {best && fmtResolution(best.width, best.height) && <span className="chip">{fmtResolution(best.width, best.height)}</span>}
          {best && <RangeChip range={best.dynamicRange} dvProfile={best.dvProfile} />}
          {e.rating != null && (
            <span className="inline-flex items-center gap-1">
              <Star size={14} className="fill-warning text-warning" />
              <span className="font-semibold text-content">{e.rating.toFixed(1)}</span>
            </span>
          )}
          {e.problem && (
            <span className="badge tint-warning" title={PROBLEM_TEXT[e.problem].detail}>
              {PROBLEM_TEXT[e.problem].label}
            </span>
          )}
        </div>

        <div className="mt-5 flex flex-wrap items-center gap-2">
          {!e.problem && (
            <Link to={`/play/${e.fileId}`} className="btn-primary !px-5 !py-2">
              <Play size={16} className="fill-current" />
              {resumeAt ? `Resume ${fmtClock(resumeAt)}` : "Play"}
            </Link>
          )}
          <button className="btn-ghost !py-2" onClick={() => void setWatched(!e.watched)}>
            {e.watched ? <EyeOff size={15} /> : <Eye size={15} />}
            {e.watched ? "Mark unwatched" : "Mark watched"}
          </button>
          {admin && (
            <button className="btn-ghost !py-2" onClick={() => setEditing(true)}>
              <Pencil size={15} /> Edit
            </button>
          )}
          {admin && <EpisodeActions item={series} e={e} onChange={reload} className="btn-ghost !px-2.5 !py-2" />}
        </div>

        {e.plot ? (
          <p className="mt-5 max-w-3xl text-sm leading-relaxed text-content-secondary">{e.plot}</p>
        ) : (
          e.problem && <p className="mt-5 max-w-3xl text-sm text-content-muted">{PROBLEM_TEXT[e.problem].detail}</p>
        )}
        <CrewLine crew={data.crew ?? []} />
      </div>

      <div className="mt-6">
        <CastRow cast={data.cast ?? []} title="Guest stars" />
      </div>

      {(prev || next) && (
        <nav className="mt-8 flex items-stretch justify-between gap-3 gutter" aria-label="Other episodes">
          <Neighbour e={prev} dir="prev" />
          <Neighbour e={next} dir="next" />
        </nav>
      )}

      {files.length > 1 && <FilesCard files={files} onChange={reload} />}
      {error && (
        <div className="gutter pt-4">
          <ErrorNote>{error}</ErrorNote>
        </div>
      )}
      {editing && admin && (
        <ItemPanel
          item={series}
          episode={{ season: e.season, episode: e.episode, label: `${number}${e.title ? ` · ${e.title}` : ""}` }}
          onClose={() => setEditing(false)}
          onChanged={() => void reload()}
        />
      )}
    </div>
  );
}

/** A link to the previous or next episode, or an empty slot that keeps the other one in place. */
function Neighbour({ e, dir }: { e: EpisodeRef | null; dir: "prev" | "next" }) {
  if (!e) return <div className="flex-1" />;
  const next = dir === "next";
  return (
    <Link to={`/episode/${e.id}`} className={`btn-ghost flex max-w-[48%] flex-1 items-center gap-2 !py-2 ${next ? "justify-end text-right" : ""}`}>
      {!next && <ChevronLeft size={16} className="shrink-0" />}
      <span className="min-w-0">
        <span className="block text-xs text-content-muted">{next ? "Next" : "Previous"}</span>
        <span className="block truncate">
          S{e.season} · E{e.episode}
          {e.title && ` · ${e.title}`}
        </span>
      </span>
      {next && <ChevronRight size={16} className="shrink-0" />}
    </Link>
  );
}
