import type { ReactNode } from "react";
import { Play, X } from "lucide-react";
import Link from "./Link";
import PosterCard from "./PosterCard";
import { backdropUrl, stillUrl } from "../lib/api";
import { fmtClock } from "../lib/format";
import type { ItemSummary, PlayInfo } from "../lib/types";

export function Row({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) {
  return (
    <section className="gutter py-3">
      <div className="mb-3 flex items-baseline justify-between">
        <h2 className="row-title">{title}</h2>
        {action}
      </div>
      {/* Inset to the page margin; the 4px of padding keeps hover shadows from being cut off. */}
      <div className="row-scroll -mx-1 flex gap-3 overflow-x-auto px-1 pb-2 pt-1 sm:gap-5">{children}</div>
    </section>
  );
}

export function PosterRow({ title, items, action }: { title: string; items: ItemSummary[]; action?: ReactNode }) {
  if (!items.length) return null;
  return (
    <Row title={title} action={action}>
      {items.map((it) => (
        <div key={it.id} className="w-28 shrink-0 sm:w-40">
          <PosterCard item={it} />
        </div>
      ))}
    </Row>
  );
}

/**
 * 16:9 resume card: episode still (or the show's backdrop) with a progress
 * strip. The picture resumes playback; the title opens the title's page.
 */
export function ContinueCard({ p, onRemove }: { p: PlayInfo; onRemove?: () => void }) {
  const img = p.hasStill ? stillUrl(p.fileId) : p.hasBackdrop ? backdropUrl({ id: p.itemId, updatedAt: p.updatedAt }) : null;
  const left = p.durationSec && p.positionSec > 0 ? Math.max(0, p.durationSec - p.positionSec) : 0;
  return (
    <div className="group/card relative w-64 shrink-0 sm:w-72">
      {onRemove && (
        <button
          type="button"
          className="card-remove"
          aria-label={`Remove ${p.title} from Continue watching`}
          title="Remove from this row (it comes back when you watch it again)"
          onClick={onRemove}
        >
          <X size={14} />
        </button>
      )}
      <Link to={`/play/${p.fileId}`} className="still-link group block" aria-label={`${p.positionSec > 0 ? "Resume" : "Play"} ${p.title}`}>
        <div className="still">
          {img ? (
            <img src={img} alt="" loading="lazy" className="absolute inset-0 h-full w-full object-cover" />
          ) : (
            <div className="poster-placeholder absolute inset-0" />
          )}
          <div className="absolute inset-0 flex items-center justify-center opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100">
            <span className="flex h-11 w-11 items-center justify-center rounded-full bg-accent text-on-accent shadow-lg">
              <Play size={20} className="translate-x-px fill-current" />
            </span>
          </div>
          {p.nextUp ? (
            <span className="art-badge scrim absolute bottom-1.5 left-1.5">Next up</span>
          ) : (
            <div className="art-progress">
              <span style={{ width: `${p.progress * 100}%` }} />
            </div>
          )}
        </div>
      </Link>
      <Link to={`/item/${p.itemId}`} className="poster-title title-link block">
        {p.title}
      </Link>
      <div className="poster-meta">
        {[p.subtitle, left ? `${fmtClock(left)} left` : null].filter(Boolean).join(" · ")}
      </div>
    </div>
  );
}
