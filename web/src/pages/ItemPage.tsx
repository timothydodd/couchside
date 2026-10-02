import { useEffect, useState } from "react";
import { ArrowLeft, Check, Eye, EyeOff, Play, RotateCcw, Star, Trash2, Wand2 } from "lucide-react";
import { CastRow, CrewLine } from "../components/Credits";
import Link from "../components/Link";
import { PosterArt } from "../components/PosterCard";
import { EmptyState, ErrorNote, Meter, Spinner } from "../components/ui";
import { api, backdropUrl, posterUrl, stillUrl, useApi } from "../lib/api";
import { fmtAirDate, fmtBytes, fmtClock, fmtResolution, fmtRuntime, titleLink } from "../lib/format";
import { canDirectPlay } from "../lib/playback";
import { PROBLEM_TEXT, type EpisodeRow, type ItemDetail, type MediaFile } from "../lib/types";
import { useIsAdmin } from "../stores/auth";
import { useRouter } from "../stores/router";
import { useStatus } from "../stores/status";
import { attempt } from "../lib/notices";

export default function ItemPage({ id }: { id: number }) {
  const { data, error, loading, reload } = useApi<ItemDetail>(`/api/items/${id}`);
  const back = useRouter((s) => s.back);
  const admin = useIsAdmin();

  if (loading && !data) {
    return (
      <div className="flex h-full items-center justify-center">
        <Spinner size={22} />
      </div>
    );
  }
  if (!data) return <EmptyState title="Title not found">{error}</EmptyState>;

  const { item, files, seasons } = data;
  const isSeries = item.kind === "series";
  const next = isSeries ? nextEpisode(seasons ?? []) : null;
  const feature = featureFiles(files);
  const extras = files.filter((f) => f.role === "extra");
  // A split movie resumes in its first unfinished part, at a time on the whole movie's clock.
  const movieAt = feature.findIndex((f) => !f.watched);
  const movieFile = feature[Math.max(0, movieAt)];
  const partOffset = feature.slice(0, Math.max(0, movieAt)).reduce((t, f) => t + (f.durationSec ?? 0), 0);
  const playFile = isSeries ? next?.fileId : movieFile?.id;
  const resumeAt = isSeries ? next?.positionSec : movieAt >= 0 ? partOffset + movieFile.positionSec : 0;
  const allWatched = item.fileCount > 0 && item.watchedCount >= item.fileCount;

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
          <img src={backdropUrl(item)} alt="" className="hero-art" />
        ) : item.hasPoster ? (
          <img src={posterUrl(item, "full")} alt="" className="absolute inset-0 h-full w-full scale-110 object-cover opacity-60 blur-2xl" />
        ) : (
          <div className="poster-placeholder absolute inset-0" />
        )}
        <div className="hero-fade absolute inset-0" />
        <button onClick={() => back(isSeries ? "/tv" : "/movies")} className="btn-ghost absolute left-4 top-4 !bg-surface/60 backdrop-blur md:left-6 md:top-5">
          <ArrowLeft size={15} /> Back
        </button>
      </section>

      <div className="relative -mt-24 flex flex-col gap-6 gutter sm:-mt-40 md:flex-row">
        {/* On phones the backdrop above is the picture; the poster would push everything below the fold. */}
        <div className="poster hidden w-44 shrink-0 self-start shadow-[var(--shadow-poster)] sm:block md:w-56">
          <PosterArt item={item} size="full" />
        </div>
        <div className="min-w-0 flex-1 md:pt-16">
          <h1 className="text-2xl font-bold leading-tight text-content sm:text-3xl md:text-4xl">{item.title}</h1>
          <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-content-secondary">
            {item.year && <span>{item.year}</span>}
            {item.rated && <span className="chip">{item.rated}</span>}
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
            <button className="btn-ghost !py-2" onClick={() => void setWatched(!allWatched)}>
              {allWatched ? <EyeOff size={15} /> : <Eye size={15} />}
              {allWatched ? "Mark unwatched" : "Mark watched"}
            </button>
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

          {admin && <MatchPanel id={item.id} status={item.matchStatus} imdbId={item.imdbId} parsed={`${item.parsedTitle}${item.parsedYear ? ` (${item.parsedYear})` : ""}`} onDone={reload} />}
        </div>
      </div>

      <div className="mt-6">
        <CastRow cast={data.cast ?? []} />
      </div>
      {isSeries && seasons && <Seasons seasons={seasons} />}
      {extras.length > 0 && <Extras title={item.title} files={extras} />}
      {!isSeries && files.length > extras.length && <FilesCard files={files.filter((f) => f.role !== "extra")} onChange={reload} />}
      {error && (
        <div className="gutter pt-4">
          <ErrorNote>{error}</ErrorNote>
        </div>
      )}
    </div>
  );
}

/** The files that make up a movie: its parts in order (the best copy of each), or its best copy. */
function featureFiles(files: MediaFile[]): MediaFile[] {
  const parts = files.filter((f) => f.role === "part" && !f.problem); // server order: part number, then biggest
  if (parts.length) return parts.filter((f, i) => i === 0 || parts[i - 1].partNo !== f.partNo);
  const copy = files.find((f) => f.role === "copy");
  return copy ? [copy] : [];
}

/** First episode that isn't finished: resume it, or start the next one. */
function nextEpisode(seasons: NonNullable<ItemDetail["seasons"]>): EpisodeRow | undefined {
  const all = seasons.flatMap((s) => s.episodes).filter((e) => !e.problem);
  return all.find((e) => !e.watched) ?? all[0];
}

function MatchPanel({ id, status, imdbId, parsed, onDone }: { id: number; status: string; imdbId: string; parsed: string; onDone: () => void }) {
  const hasProvider = !!useStatus((s) => s.status?.providers.length);
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("");
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // While a match is pending, poll so the page fills in on its own.
  useEffect(() => {
    if (status !== "pending") return;
    const t = setInterval(onDone, 2500);
    return () => clearInterval(t);
  }, [status, onDone]);

  if (!hasProvider) return null;

  const submit = async (imdb: string) => {
    setBusy(true);
    setErr(null);
    try {
      await api(`/api/items/${id}/match`, { method: "POST", json: { imdbId: imdb } });
      setOpen(false);
      setValue("");
      onDone();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="mt-5">
      {!open ? (
        <div className="flex flex-wrap items-center gap-2 text-xs text-content-muted">
          {status === "unmatched" && <span className="tint-warning rounded px-1.5 py-0.5 font-semibold">Unmatched</span>}
          <span>
            Read from files as <span className="text-content-secondary">{parsed}</span>
            {imdbId && (
              <>
                {" · "}
                <a href={titleLink(imdbId) ?? undefined} target="_blank" rel="noreferrer" className="hover:text-accent">
                  {imdbId}
                </a>
              </>
            )}
          </span>
          <button className="btn-quiet !text-xs" onClick={() => setOpen(true)}>
            <Wand2 size={13} /> Fix match
          </button>
        </div>
      ) : (
        <div className="card max-w-xl p-3">
          <label className="field-label" htmlFor="imdb">
            IMDb or TMDB id or URL
          </label>
          <div className="flex gap-2">
            <input
              id="imdb"
              className="field min-w-0 flex-1"
              placeholder="tt0133093, or an imdb.com or themoviedb.org link"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && value.trim() && void submit(value)}
              autoFocus
            />
            <button className="btn-primary" disabled={busy || !value.trim()} onClick={() => void submit(value)}>
              <Check size={15} /> Match
            </button>
          </div>
          <div className="mt-2 flex items-center gap-2">
            <button className="btn-quiet !text-xs" disabled={busy} onClick={() => void submit("")}>
              <RotateCcw size={13} /> Retry automatic match
            </button>
            <button className="btn-quiet !text-xs ml-auto" onClick={() => setOpen(false)}>
              Cancel
            </button>
          </div>
          {err && <div className="mt-2 text-xs text-critical">{err}</div>}
        </div>
      )}
    </div>
  );
}

function Seasons({ seasons }: { seasons: NonNullable<ItemDetail["seasons"]> }) {
  const firstUnwatched = seasons.find((s) => s.episodes.some((e) => !e.watched))?.season ?? seasons[0]?.season;
  const [season, setSeason] = useState(firstUnwatched);
  const current = seasons.find((s) => s.season === season) ?? seasons[0];
  if (!current) return null;
  return (
    <section className="mt-10 gutter">
      <div className="flex gap-1 overflow-x-auto border-b border-border-light">
        {seasons.map((s) => (
          <button key={s.season} onClick={() => setSeason(s.season)} className={`navtab !text-sm ${s.season === current.season ? "navtab-active" : ""}`}>
            {s.season === 0 ? "Specials" : `Season ${s.season}`}
          </button>
        ))}
      </div>
      <ol className="mt-2 flex flex-col">
        {current.episodes.map((e) => (
          <EpisodeItem key={e.id} e={e} />
        ))}
      </ol>
    </section>
  );
}

function EpisodeItem({ e }: { e: EpisodeRow }) {
  return (
    <StillRow file={e}>
      <div className="flex items-baseline gap-2">
        {e.airDate ? (
          <span className="shrink-0 text-xs font-semibold tabular-nums text-content-muted">{fmtAirDate(e.airDate)}</span>
        ) : (
          <span className="text-xs font-semibold tabular-nums text-content-muted">E{e.episode}</span>
        )}
        <span className="truncate text-sm font-medium text-content">{e.title || (e.airDate ? "" : `Episode ${e.episode}`)}</span>
      </div>
      <div className="mt-0.5 flex flex-wrap gap-x-3 text-xs text-content-muted">
        {e.released && <span>{e.released}</span>}
        {e.durationSec && <span>{fmtRuntime(e.durationSec)}</span>}
        {e.rating != null && (
          <span className="inline-flex items-center gap-1">
            <Star size={11} className="fill-warning text-warning" />
            {e.rating.toFixed(1)}
          </span>
        )}
      </div>
    </StillRow>
  );
}

/** A movie's bonus material, listed like episodes and titled "Movie - Extra". */
function Extras({ title, files }: { title: string; files: MediaFile[] }) {
  return (
    <section className="mt-10 gutter">
      <h2 className="row-title mb-1">Extras</h2>
      <ol className="flex flex-col">
        {files.map((f) => (
          <StillRow key={f.id} file={{ ...f, fileId: f.id }}>
            <div className="truncate text-sm font-medium text-content">
              <span className="text-content-muted">{title} - </span>
              {f.extraTitle}
            </div>
            {f.durationSec && <div className="mt-0.5 text-xs text-content-muted">{fmtRuntime(f.durationSec)}</div>}
          </StillRow>
        ))}
      </ol>
    </section>
  );
}

/** A playable row with the file's frame on the left: episodes and extras. */
function StillRow({
  file: e,
  children,
}: {
  file: Pick<EpisodeRow, "fileId" | "hasStill" | "durationSec" | "positionSec" | "watched" | "problem">;
  children: React.ReactNode;
}) {
  const progress = e.durationSec && !e.watched ? (e.positionSec / e.durationSec) * 100 : 0;
  return (
    <li>
      <Link to={`/play/${e.fileId}`} className="still-link group flex items-center gap-4 rounded-lg px-2 py-2.5 transition-colors hover:bg-muted">
        <div className="still w-44 shrink-0">
          {e.hasStill ? (
            <img src={stillUrl(e.fileId)} alt="" loading="lazy" className="absolute inset-0 h-full w-full object-cover" />
          ) : (
            <div className="poster-placeholder absolute inset-0" />
          )}
          <div className="absolute inset-0 flex items-center justify-center opacity-0 transition-opacity group-hover:opacity-100">
            <span className="flex h-9 w-9 items-center justify-center rounded-full bg-accent text-on-accent">
              <Play size={16} className="translate-x-px fill-current" />
            </span>
          </div>
          {progress > 0 && (
            <div className="art-progress">
              <span style={{ width: `${progress}%` }} />
            </div>
          )}
        </div>
        <div className="min-w-0 flex-1">{children}</div>
        {e.problem && (
          <span className="tint-warning shrink-0 rounded px-1.5 py-0.5 text-[11px] font-semibold" title={PROBLEM_TEXT[e.problem].detail}>
            {PROBLEM_TEXT[e.problem].label}
          </span>
        )}
        {e.watched && (
          <span className="tint-good inline-flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-[11px] font-semibold">
            <Check size={12} /> Watched
          </span>
        )}
      </Link>
    </li>
  );
}

/** How this browser will play a file. */
function PlaybackChip({ f }: { f: MediaFile }) {
  if (f.problem)
    return (
      <span className="tint-warning rounded px-1.5 py-0.5 text-[11px] font-semibold" title={PROBLEM_TEXT[f.problem].detail}>
        {PROBLEM_TEXT[f.problem].label}
      </span>
    );
  if (canDirectPlay(f)) return <span className="tint-good rounded px-1.5 py-0.5 text-[11px] font-semibold">Direct play</span>;
  if (f.optimized) return <span className="tint-good rounded px-1.5 py-0.5 text-[11px] font-semibold">Optimized</span>;
  return (
    <span className="tint-info rounded px-1.5 py-0.5 text-[11px] font-semibold" title="The browser can't play this format as is; it will be converted while you watch">
      Transcode
    </span>
  );
}

function FilesCard({ files, onChange }: { files: MediaFile[]; onChange: () => void }) {
  const admin = useIsAdmin();
  const dropOptimized = attempt("Couldn't delete the optimized copy", async (id: number) => {
    if (!confirm("Delete the optimized copy? Playback falls back to the original file or a server stream.")) return;
    await api(`/api/files/${id}/optimized`, { method: "DELETE" });
    onChange();
  });
  return (
    <section className="mt-10 gutter">
      <h2 className="row-title mb-3">{files.length === 1 ? "File" : `Files (${files.length})`}</h2>
      <div className="card overflow-x-auto">
        <table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Quality</th>
              <th className="hidden sm:table-cell">Video</th>
              <th className="hidden sm:table-cell">Audio</th>
              <th className="hidden sm:table-cell">Length</th>
              <th className="text-right">Size</th>
              <th>Playback</th>
              <th>Progress</th>
            </tr>
          </thead>
          <tbody>
            {files.map((f) => (
              <tr key={f.id}>
                <td className="mono max-w-xs truncate" title={f.path}>
                  {f.role === "part" && <span className="chip mr-1.5">Part {f.partNo}</span>}
                  <Link to={`/play/${f.id}`} className="hover:text-accent">
                    {f.path}
                  </Link>
                </td>
                <td>{fmtResolution(f.width, f.height) && <span className="chip">{fmtResolution(f.width, f.height)}</span>}</td>
                <td className="mono hidden text-content-secondary sm:table-cell">{f.videoCodec || "?"}</td>
                <td className="mono hidden text-content-secondary sm:table-cell">
                  {f.audioCodec || "?"}
                  {f.audioTracks > 1 && <span className="text-content-muted"> +{f.audioTracks - 1}</span>}
                </td>
                <td className="hidden tabular-nums text-content-secondary sm:table-cell">{fmtRuntime(f.durationSec)}</td>
                <td className="text-right tabular-nums text-content-secondary">{fmtBytes(f.size)}</td>
                <td>
                  <span className="inline-flex items-center gap-1">
                    <PlaybackChip f={f} />
                    {f.optimized && admin && (
                      <button className="btn-quiet !p-1 hover:!text-critical" title="Delete the optimized copy" onClick={() => void dropOptimized(f.id)}>
                        <Trash2 size={12} />
                      </button>
                    )}
                  </span>
                </td>
                <td className="w-32">
                  {f.watched ? (
                    <span className="text-xs text-good">Watched</span>
                  ) : f.durationSec && f.positionSec > 0 ? (
                    <Meter value={(f.positionSec / f.durationSec) * 100} />
                  ) : (
                    <span className="text-xs text-content-muted">—</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
