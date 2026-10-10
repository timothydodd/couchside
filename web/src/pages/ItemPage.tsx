import { useEffect, useRef, useState } from "react";
import { Bookmark, BookmarkCheck, Eye, EyeOff, Pencil, Play, Star, Wand2 } from "lucide-react";
import EpisodeCard from "../components/EpisodeCard";
import FilesCard from "../components/item/FilesCard";
import ItemPanel, { type PanelSection } from "../components/manage/ItemPanel";
import { CastRow, CrewLine } from "../components/Credits";
import Link from "../components/Link";
import FadeImg from "../components/FadeImg";
import { PosterArt } from "../components/PosterCard";
import { BackButton, EmptyState, ErrorNote, Loading } from "../components/ui";
import { api, backdropUrl, posterUrl, useApi } from "../lib/api";
import { usePhone } from "../lib/media";
import { useScrollEdges } from "../lib/scroll";
import { fmtAirDate, fmtBytes, fmtClock, fmtResolution, fmtRuntime, titleLink } from "../lib/format";
import { EpisodeActions, ExtraActions, TitleActions } from "../components/item/AdminMenus";
import type { EpisodeRow, ItemDetail, MediaFile } from "../lib/types";
import { useIsAdmin } from "../stores/auth";
import { useStatus } from "../stores/status";
import { featureFiles, nextEpisode } from "../lib/items";
import { attempt } from "../lib/notices";
import RangeChip from "../components/RangeChip";
import { rangeLabel } from "../lib/quality";
import { useTitle } from "../lib/title";

export default function ItemPage({ id, season, edit }: { id: number; season?: number; edit?: boolean }) {
  const { data, error, loading, reload } = useApi<ItemDetail>(`/api/items/${id}`);
  useTitle(data && (season !== undefined && data.item.kind === "series" ? `${data.item.title} · Season ${season}` : data.item.title));
  const admin = useIsAdmin();
  // The Edit panel: open from the button, from "Fix match", or by ?edit=1 (the library table links here).
  const [editing, setEditing] = useState<PanelSection | null>(edit ? "details" : null);

  if (loading && !data) {
    return (
      <Loading fill />
    );
  }
  if (!data) {
    // Only a 404 means it isn't there; anything else is a failed request.
    const missing = !error || /not found/i.test(error);
    return (
      <EmptyState title={missing ? "Title not found" : "Couldn't load this title"}>
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

  const { item, files, seasons } = data;
  const isSeries = item.kind === "series";
  const next = isSeries ? nextEpisode(seasons ?? []) : null;
  // A movie with more than one copy (4K and 1080p, two cuts): the profile's choice, or the biggest.
  const versions = isSeries ? [] : files.filter((f) => f.role === "copy" && !f.problem);
  const chosen = versions.find((f) => f.id === data.versionFileId);
  const feature = featureFiles(files, chosen);
  const setVersion = attempt("Couldn't change the version", async (fileId: number) => {
    await api(`/api/items/${id}/version`, { method: "PUT", json: { fileId } });
    await reload();
  });
  const extras = files.filter((f) => f.role === "extra");
  // A split movie resumes in its first unfinished part, at a time on the whole movie's clock.
  const movieAt = feature.findIndex((f) => !f.watched);
  const movieFile = feature[Math.max(0, movieAt)];
  const partOffset = feature.slice(0, Math.max(0, movieAt)).reduce((t, f) => t + (f.durationSec ?? 0), 0);
  const playFile = isSeries ? next?.fileId : movieFile?.id;
  const resumeAt = isSeries ? next?.positionSec : movieAt >= 0 ? partOffset + movieFile.positionSec : 0;
  const allWatched = item.fileCount > 0 && item.watchedCount >= item.fileCount;

  const setListed = attempt("Couldn't change your list", async (on: boolean) => {
    await api(`/api/items/${id}/watchlist`, { method: on ? "PUT" : "DELETE" });
    await reload();
  });
  const setWatched = attempt("Couldn't change watched", async (watched: boolean) => {
    await api(`/api/items/${item.id}/watched`, { method: "POST", json: { watched } });
    await reload();
  });

  const runtime = item.runtimeMin ? fmtRuntime(item.runtimeMin * 60) : fmtRuntime(feature.reduce((t, f) => t + (f.durationSec ?? 0), 0) || null);

  return (
    <div className="pb-10">
      {/* Backdrop: the grabbed frame, or a blown-up blurred poster as a fallback. */}
      <section className="relative h-[40vh] min-h-64 max-h-[460px] overflow-hidden">
        {item.hasBackdrop ? (
          <FadeImg src={backdropUrl(item)} alt="" className="hero-art" />
        ) : item.hasPoster ? (
          <FadeImg src={posterUrl(item, "full")} alt="" className="absolute inset-0 h-full w-full scale-110 object-cover blur-2xl" style={{ "--img-opacity": 0.6 } as React.CSSProperties} />
        ) : (
          <div className="poster-placeholder absolute inset-0" />
        )}
        <div className="hero-fade absolute inset-0" />
        <BackButton fallback={isSeries ? "/tv" : "/movies"} overlay />
      </section>

      <div className="relative -mt-24 flex flex-col gap-6 gutter sm:-mt-40 md:flex-row">
        {/* On phones the backdrop above is the picture; the poster would push everything below the fold. */}
        <div className="poster hidden w-44 shrink-0 self-start shadow-[var(--shadow-poster)] sm:block md:w-56" data-hero={item.id} data-hero-page>
          <PosterArt item={item} size="full" />
        </div>
        <div className="min-w-0 flex-1 md:pt-16">
          <h1 className="text-2xl font-bold leading-tight text-content sm:text-3xl md:text-4xl">{item.title}</h1>
          <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-content-secondary">
            {item.year && <span>{item.year}</span>}
            {item.rated && <span className="chip">{item.rated}</span>}
            <RangeChip range={item.dynamicRange} />
            {!isSeries && runtime && <span>{runtime}</span>}
            {feature.length > 1 && <span>{feature.length} parts</span>}
            {isSeries && seasons && (
              <span>
                {seasons.length} season{seasons.length === 1 ? "" : "s"} · {item.fileCount} episode{item.fileCount === 1 ? "" : "s"}
              </span>
            )}
            {item.rating != null && (
              <span className="inline-flex items-center gap-1">
                <Star size={14} className="fill-warning text-warning" />
                <span className="font-semibold text-content">{item.rating.toFixed(1)}</span>
                <span className="text-content-muted">{item.matchProvider === "tmdb" ? "TMDB" : "IMDb"}</span>
              </span>
            )}
          </div>
          {item.genres.length > 0 && (
            <div className="mt-3 flex flex-wrap gap-1.5">
              {item.genres.map((g) => (
                <span key={g} className="chip">
                  {g}
                </span>
              ))}
            </div>
          )}

          <div className="mt-5 flex flex-wrap items-center gap-2">
            {playFile && (
              <Link to={`/play/${playFile}`} className="btn-primary !px-5 !py-2">
                <Play size={16} className="fill-current" />
                {resumeAt && resumeAt > 30
                  ? `Resume ${fmtClock(resumeAt)}`
                  : isSeries && next
                    ? next.airDate
                      ? `Play ${fmtAirDate(next.airDate).split(" · ")[0]}`
                      : `Play S${next.season} · E${next.episode}`
                    : "Play"}
              </Link>
            )}
            {versions.length > 1 && feature.length === 1 && (
              <select
                className="field !py-2"
                aria-label="Version to watch"
                title="Which copy plays. Your choice is remembered for this title."
                value={feature[0].id}
                onChange={(e) => void setVersion(Number(e.target.value))}
              >
                {versions.map((f) => (
                  <option key={f.id} value={f.id}>
                    {versionLabel(f)}
                  </option>
                ))}
              </select>
            )}
            <button className="btn-ghost !py-2" aria-pressed={item.inWatchlist} onClick={() => void setListed(!item.inWatchlist)}>
              {item.inWatchlist ? <BookmarkCheck size={15} className="text-accent" /> : <Bookmark size={15} />}
              {item.inWatchlist ? "On my list" : "My list"}
            </button>
            <button className="btn-ghost !py-2" onClick={() => void setWatched(!allWatched)}>
              {allWatched ? <EyeOff size={15} /> : <Eye size={15} />}
              {allWatched ? "Mark unwatched" : "Mark watched"}
            </button>
            {admin && (
              <button className="btn-ghost !py-2" onClick={() => setEditing("details")}>
                <Pencil size={15} /> Edit
              </button>
            )}
            {admin && <TitleActions item={item} onChange={reload} />}
          </div>

          {item.plot ? (
            <p className="mt-5 max-w-3xl text-sm leading-relaxed text-content-secondary">{item.plot}</p>
          ) : (
            item.matchStatus !== "matched" && (
              <p className="mt-5 max-w-3xl text-sm text-content-muted">
                {item.matchStatus === "pending" ? "Looking up details…" : "No details found for this title yet."}
              </p>
            )
          )}

          <CrewLine crew={data.crew ?? []} />

          {admin && <MatchLine item={item} onFix={() => setEditing("match")} onDone={reload} />}
        </div>
      </div>

      <div className="mt-6">
        <CastRow cast={data.cast ?? []} />
      </div>
      {isSeries && seasons && <Seasons item={item} seasons={seasons} open={season} admin={admin} onChange={reload} />}
      {extras.length > 0 && <Extras item={item} files={extras} admin={admin} onChange={reload} />}
      {!isSeries && files.length > extras.length && <FilesCard files={files.filter((f) => f.role !== "extra")} onChange={reload} />}
      {error && (
        <div className="gutter pt-4">
          <ErrorNote>{error}</ErrorNote>
        </div>
      )}
      {editing && admin && <ItemPanel item={item} section={editing} onClose={() => setEditing(null)} onChanged={() => void reload()} />}
    </div>
  );
}

/** A copy of a movie, as the version chooser names it: "4K · Dolby Vision · HEVC · Extended · 54 GB". */
function versionLabel(f: MediaFile): string {
  return [fmtResolution(f.width, f.height), rangeLabel(f.dynamicRange), f.videoCodec.toUpperCase(), f.edition, fmtBytes(f.size)].filter(Boolean).join(" · ");
}

/** For admins: where the match stands, with "Fix match" opening the Edit panel on it. Polls while a match is pending. */
function MatchLine({ item, onFix, onDone }: { item: ItemDetail["item"]; onFix: () => void; onDone: () => void }) {
  const hasProvider = !!useStatus((s) => s.status?.providers.length);
  useEffect(() => {
    if (item.matchStatus !== "pending") return;
    const t = setInterval(onDone, 2500);
    return () => clearInterval(t);
  }, [item.matchStatus, onDone]);
  if (!hasProvider) return null;
  const parsed = `${item.parsedTitle}${item.parsedYear ? ` (${item.parsedYear})` : ""}`;
  return (
    <div className="mt-5 flex flex-wrap items-center gap-2 text-xs text-content-muted">
      {item.matchStatus === "unmatched" && <span className="badge tint-warning">Unmatched</span>}
      {item.matchStatus === "pending" && <span className="badge tint-info">Matching…</span>}
      <span>
        Read from files as <span className="text-content-secondary">{parsed}</span>
        {item.imdbId && (
          <>
            {" · "}
            <a href={titleLink(item.imdbId) ?? undefined} target="_blank" rel="noreferrer" className="hover:text-accent">
              {item.imdbId}
            </a>
          </>
        )}
      </span>
      <button className="btn-quiet !text-xs" onClick={onFix}>
        <Wand2 size={13} /> Fix match
      </button>
    </div>
  );
}

function Seasons({
  item,
  seasons,
  open,
  admin,
  onChange,
}: {
  item: ItemDetail["item"];
  seasons: NonNullable<ItemDetail["seasons"]>;
  /** The season to open on (from ?season=, coming back from an episode); else the first with something unwatched. */
  open?: number;
  admin: boolean;
  onChange: () => void;
}) {
  const firstUnwatched = seasons.find((s) => s.episodes.some((e) => !e.watched))?.season ?? seasons[0]?.season;
  const [season, setSeason] = useState(open ?? firstUnwatched);
  const current = seasons.find((s) => s.season === season) ?? seasons[0];
  const tabs = useRef<HTMLDivElement>(null);
  useScrollEdges(tabs);
  // A show opened on a later season: bring its tab into view.
  useEffect(() => {
    tabs.current?.querySelector<HTMLElement>(".navtab-active")?.scrollIntoView({ block: "nearest", inline: "nearest" });
  }, [season]);
  if (!current) return null;
  return (
    <section className="mt-10 gutter">
      <div ref={tabs} className="row-scroll flex gap-1 overflow-x-auto border-b border-border-light">
        {seasons.map((s) => (
          <button key={s.season} type="button" onClick={() => setSeason(s.season)} className={`navtab shrink-0 !text-sm ${s.season === current.season ? "navtab-active" : ""}`}>
            {s.season === 0 ? "Specials" : `Season ${s.season}`}
          </button>
        ))}
      </div>
      <TileList>
        {current.episodes.map((e) => (
          <li key={e.id}>
            <EpisodeTile e={e} actions={admin ? <EpisodeActions item={item} e={e} onChange={onChange} /> : undefined} />
          </li>
        ))}
      </TileList>
    </section>
  );
}

/** Episode and extra tiles: a grid from tablet width up, a list on phones. */
function TileList({ children }: { children: React.ReactNode }) {
  const phone = usePhone();
  return <ol className={phone ? "mt-2 flex flex-col" : "mt-4 grid grid-cols-[repeat(auto-fill,minmax(220px,1fr))] gap-x-4 gap-y-5"}>{children}</ol>;
}

function EpisodeTile({ e, actions }: { e: EpisodeRow; actions?: React.ReactNode }) {
  const phone = usePhone();
  const name = e.airDate ? fmtAirDate(e.airDate) : `S${e.season} E${e.episode}`;
  return (
    <EpisodeCard
      file={e}
      layout={phone ? "row" : "grid"}
      eyebrow={e.airDate ? fmtAirDate(e.airDate) : `E${e.episode}`}
      title={e.title || (e.airDate ? "" : `Episode ${e.episode}`)}
      to={`/episode/${e.id}`}
      playLabel={`Play ${name}${e.title ? ` ${e.title}` : ""}`}
      actions={actions}
      meta={
        (e.released || e.durationSec || e.rating != null) && (
          <>
            {e.released && <span>{e.released}</span>}
            {e.durationSec && <span>{fmtRuntime(e.durationSec)}</span>}
            {e.rating != null && (
              <span className="inline-flex items-center gap-1">
                <Star size={11} className="fill-warning text-warning" />
                {e.rating.toFixed(1)}
              </span>
            )}
          </>
        )
      }
    />
  );
}

/** A movie's bonus material, tiled like episodes. */
function Extras({ item, files, admin, onChange }: { item: ItemDetail["item"]; files: MediaFile[]; admin: boolean; onChange: () => void }) {
  const phone = usePhone();
  return (
    <section className="mt-10 gutter">
      <h2 className="row-title">Extras</h2>
      <TileList>
        {files.map((f) => (
          <li key={f.id}>
            <EpisodeCard
              file={{ ...f, fileId: f.id }}
              layout={phone ? "row" : "grid"}
              title={f.extraTitle || "Extra"}
              playLabel={`Play ${f.extraTitle || "extra"}`}
              meta={f.durationSec ? <span>{fmtRuntime(f.durationSec)}</span> : undefined}
              actions={admin ? <ExtraActions item={item} f={f} onChange={onChange} /> : undefined}
            />
          </li>
        ))}
      </TileList>
    </section>
  );
}
