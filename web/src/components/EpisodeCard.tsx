import type { ReactNode } from "react";
import { Check, Play } from "lucide-react";
import Link from "./Link";
import FadeImg from "./FadeImg";
import { stillUrl } from "../lib/api";
import { PROBLEM_TEXT, type EpisodeRow } from "../lib/types";

export type EpisodeCardFile = Pick<EpisodeRow, "fileId" | "hasStill" | "durationSec" | "positionSec" | "watched" | "problem">;

/**
 * A 16:9 tile for an episode or a movie's extra. The picture plays it (a
 * file with a problem can't be played, so its picture opens the page too);
 * the caption opens its page when `to` is given. `layout="row"` puts the
 * caption beside a small still, for phones and lists.
 */
export default function EpisodeCard({
  file: f,
  eyebrow,
  title,
  meta,
  to,
  actions,
  layout = "grid",
  playLabel,
}: {
  file: EpisodeCardFile;
  /** Above the title: "E4", or an air date. */
  eyebrow?: ReactNode;
  title: ReactNode;
  /** Under the title: date, runtime, rating. */
  meta?: ReactNode;
  /** The page the caption opens. */
  to?: string;
  /** The admin's "⋯", shown on the still. */
  actions?: ReactNode;
  layout?: "grid" | "row";
  /** For screen readers: "Play S2 E4 Title". */
  playLabel: string;
}) {
  const progress = f.durationSec && !f.watched ? (f.positionSec / f.durationSec) * 100 : 0;
  const playable = !f.problem;
  const pictureTo = playable ? `/play/${f.fileId}` : to;
  const row = layout === "row";

  const picture = (
    <div className={`still ${row ? "w-40 shrink-0 sm:w-44" : ""}`}>
      {f.hasStill ? (
        <FadeImg src={stillUrl(f.fileId)} alt="" loading="lazy" decoding="async" className="absolute inset-0 h-full w-full object-cover" />
      ) : (
        <div className="poster-placeholder absolute inset-0" />
      )}
      {playable && (
        <div className="absolute inset-0 flex items-center justify-center opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100">
          <span className={`flex items-center justify-center rounded-full bg-accent text-on-accent shadow-lg ${row ? "h-9 w-9" : "h-11 w-11"}`}>
            <Play size={row ? 16 : 20} className="translate-x-px fill-current" />
          </span>
        </div>
      )}
      <div className="absolute right-1.5 top-1.5 flex flex-col items-end gap-1">
        {f.problem && (
          <span className="art-badge tint-warning" title={PROBLEM_TEXT[f.problem].detail}>
            {PROBLEM_TEXT[f.problem].label}
          </span>
        )}
        {f.watched && (
          <span className="art-badge tint-good !bg-good/90 !text-on-accent" title="Watched">
            <Check size={12} strokeWidth={3} />
          </span>
        )}
      </div>
      {progress > 0 && (
        <div className="art-progress">
          <span style={{ width: `${progress}%` }} />
        </div>
      )}
    </div>
  );

  const caption = (
    <>
      <div className="flex items-baseline gap-2">
        {eyebrow && <span className="shrink-0 text-xs font-semibold tabular-nums text-content-muted">{eyebrow}</span>}
        <span className="truncate text-sm font-medium text-content">{title}</span>
      </div>
      {meta && <div className="mt-0.5 flex flex-wrap gap-x-3 text-xs tabular-nums text-content-muted">{meta}</div>}
    </>
  );

  return (
    <div className={`group/card relative ${row ? "flex items-center gap-4 rounded-lg px-2 py-2.5" : ""}`}>
      {actions && <div className={`absolute z-10 ${row ? "left-3 top-3.5" : "left-1.5 top-1.5"} md:opacity-0 md:focus-within:opacity-100 md:group-hover/card:opacity-100`}>{actions}</div>}
      {pictureTo ? (
        <Link to={pictureTo} className={`still-link group block ${row ? "shrink-0" : ""}`} aria-label={playable ? playLabel : undefined}>
          {picture}
        </Link>
      ) : (
        picture
      )}
      {to ? (
        <Link to={to} className={`title-link block min-w-0 ${row ? "flex-1" : "mt-2"}`}>
          {caption}
        </Link>
      ) : (
        <div className={`min-w-0 ${row ? "flex-1" : "mt-2"}`}>{caption}</div>
      )}
    </div>
  );
}
