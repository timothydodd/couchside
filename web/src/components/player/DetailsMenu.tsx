import { useEffect, useRef, useState } from "react";
import { Check, ExternalLink, Play, Star, X } from "lucide-react";
import FadeImg from "../FadeImg";
import { posterUrl, stillUrl, useApi } from "../../lib/api";
import { useDialog } from "../../lib/dialog";
import { fmtRuntime } from "../../lib/format";
import { PROBLEM_TEXT, type FileProblem, type ItemDetail } from "../../lib/types";

/** What a row shows of an episode or an extra. */
interface RowFile {
  fileId: number;
  hasStill: boolean;
  durationSec: number | null;
  positionSec: number;
  watched: boolean;
  problem: FileProblem;
  rating?: number | null;
}

/**
 * The title's details over the player, opened from the title in the top bar:
 * poster, plot and, for a show, its seasons and episodes with the one playing
 * marked. Picking an episode (or an extra) plays it. The player pauses while
 * it's open and carries on when it closes.
 */
export default function DetailsMenu({
  itemId,
  current,
  onPlay,
  onOpen,
  onClose,
}: {
  itemId: number;
  current: number; // file id
  onPlay: (fileId: number) => void;
  /** Leave the player for the title's page. */
  onOpen: () => void;
  onClose: () => void;
}) {
  const { data, error } = useApi<ItemDetail>(`/api/items/${itemId}`);
  const dialog = useRef<HTMLDivElement>(null);
  useDialog(dialog, onClose);
  const item = data?.item;
  const isSeries = item?.kind === "series";
  const extras = data?.files.filter((f) => f.role === "extra") ?? [];
  const meta = item ? [item.year, item.rated, !isSeries && fmtRuntime((item.runtimeMin ?? 0) * 60), item.genres.slice(0, 3).join(", ")].filter(Boolean) : [];

  return (
    <div
      className="anim-pop card absolute inset-x-3 top-16 bottom-3 z-30 mx-auto flex max-w-3xl origin-top flex-col overflow-hidden text-sm shadow-[var(--shadow-md)] md:inset-x-6 md:top-20 md:bottom-20"
      onClick={(e) => e.stopPropagation()}
      onDoubleClick={(e) => e.stopPropagation()}
      role="dialog"
      aria-modal="true"
      aria-label="Details"
      ref={dialog}
    >
      <div className="flex items-start gap-4 p-4 pb-3">
        {item && (
          <div className="poster hidden w-20 shrink-0 sm:block">
            {item.hasPoster ? <FadeImg src={posterUrl(item)} alt="" className="absolute inset-0 h-full w-full object-cover" /> : <div className="poster-placeholder absolute inset-0" />}
          </div>
        )}
        <div className="min-w-0 flex-1">
          <div className="flex items-start gap-2">
            <h2 className="min-w-0 flex-1 text-lg font-semibold leading-tight text-content">{item?.title ?? (error ? "Couldn't load the details" : "Loading…")}</h2>
            <button type="button" className="btn-quiet !p-1" onClick={onClose} aria-label="Close">
              <X size={16} />
            </button>
          </div>
          {meta.length > 0 && <div className="mt-0.5 text-xs text-content-muted">{meta.join(" · ")}</div>}
          {item?.plot && <p className="mt-2 line-clamp-3 text-xs leading-relaxed text-content-secondary">{item.plot}</p>}
          {item && (
            <button type="button" className="btn-quiet mt-2 !px-2 !text-xs" onClick={onOpen}>
              <ExternalLink size={13} /> Open title page
            </button>
          )}
        </div>
      </div>
      {isSeries && data?.seasons && data.seasons.length > 0 && <Seasons seasons={data.seasons} current={current} onPlay={onPlay} />}
      {!isSeries && extras.length > 0 && (
        <>
          <div className="border-t border-border-light px-4 pb-1 pt-3">
            <span className="card-title !text-content-secondary">Extras</span>
          </div>
          <List>
            {extras.map((f) => (
              <Row key={f.id} file={{ ...f, fileId: f.id }} playing={f.id === current} title={f.extraTitle || "Extra"} onPlay={onPlay} />
            ))}
          </List>
        </>
      )}
    </div>
  );
}

function Seasons({ seasons, current, onPlay }: { seasons: NonNullable<ItemDetail["seasons"]>; current: number; onPlay: (fileId: number) => void }) {
  // Open on the season of the episode playing.
  const playing = seasons.find((s) => s.episodes.some((e) => e.fileId === current)) ?? seasons[0];
  const [season, setSeason] = useState(playing.season);
  const shown = seasons.find((s) => s.season === season) ?? playing;
  const tabs = useRef<HTMLDivElement>(null);
  useEffect(() => {
    tabs.current?.querySelector<HTMLElement>(".navtab-active")?.scrollIntoView({ block: "nearest", inline: "nearest" });
  }, [season]);
  return (
    <>
      {seasons.length > 1 && (
        <div ref={tabs} className="row-scroll flex shrink-0 gap-1 overflow-x-auto border-t border-b border-border-light px-2">
          {seasons.map((s) => (
            <button key={s.season} type="button" onClick={() => setSeason(s.season)} className={`navtab shrink-0 ${s.season === shown.season ? "navtab-active" : ""}`}>
              {s.season === 0 ? "Specials" : `Season ${s.season}`}
            </button>
          ))}
        </div>
      )}
      <List key={shown.season}>
        {shown.episodes.map((e) => (
          <Row key={e.id} file={e} playing={e.fileId === current} eyebrow={`E${e.episode}`} title={e.title || `Episode ${e.episode}`} onPlay={onPlay} />
        ))}
      </List>
    </>
  );
}

/** The scrolling episode or extra list, starting at the one playing. */
function List({ children }: { children: React.ReactNode }) {
  const list = useRef<HTMLDivElement>(null);
  useEffect(() => {
    list.current?.querySelector<HTMLElement>("[data-current]")?.scrollIntoView({ block: "center" });
  }, []);
  return (
    <div ref={list} className="min-h-0 flex-1 overflow-y-auto p-1.5">
      {children}
    </div>
  );
}

function Row({
  file: f,
  playing,
  eyebrow,
  title,
  onPlay,
}: {
  file: RowFile;
  playing: boolean;
  eyebrow?: string;
  title: string;
  onPlay: (fileId: number) => void;
}) {
  const { fileId, rating } = f;
  const progress = f.durationSec && !f.watched ? (f.positionSec / f.durationSec) * 100 : 0;
  const playable = !f.problem && fileId > 0;
  return (
    <button
      type="button"
      className={`flex w-full items-center gap-3 rounded px-2 py-1.5 text-left ${playing ? "bg-accent/15" : playable ? "hover:bg-muted" : "opacity-60"}`}
      onClick={() => playable && !playing && onPlay(fileId)}
      disabled={!playable}
      aria-current={playing || undefined}
      data-current={playing || undefined}
    >
      <span className="still w-24 shrink-0 sm:w-28">
        {f.hasStill ? <FadeImg src={stillUrl(fileId)} alt="" loading="lazy" decoding="async" className="absolute inset-0 h-full w-full object-cover" /> : <span className="poster-placeholder absolute inset-0" />}
        {playing && (
          <span className="absolute inset-0 flex items-center justify-center bg-black/40">
            <Play size={16} className="fill-current text-accent" />
          </span>
        )}
        {progress > 0 && (
          <span className="art-progress">
            <span style={{ width: `${progress}%` }} />
          </span>
        )}
      </span>
      <span className="min-w-0 flex-1">
        <span className="flex items-baseline gap-2">
          {eyebrow && <span className="shrink-0 text-xs font-semibold tabular-nums text-content-muted">{eyebrow}</span>}
          <span className={`truncate font-medium ${playing ? "text-accent" : "text-content"}`}>{title}</span>
        </span>
        <span className="mt-0.5 flex flex-wrap items-center gap-x-3 text-xs tabular-nums text-content-muted">
          {f.durationSec ? <span>{fmtRuntime(f.durationSec)}</span> : null}
          {rating != null && (
            <span className="inline-flex items-center gap-1">
              <Star size={11} className="fill-warning text-warning" /> {rating.toFixed(1)}
            </span>
          )}
          {f.problem && <span className="text-warning">{PROBLEM_TEXT[f.problem].label}</span>}
          {f.watched && (
            <span className="inline-flex items-center gap-1 text-good">
              <Check size={12} strokeWidth={3} /> Watched
            </span>
          )}
        </span>
      </span>
    </button>
  );
}
