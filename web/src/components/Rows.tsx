import type { ReactNode } from "react";
import { Play } from "lucide-react";
import Link from "./Link";
import PosterCard from "./PosterCard";
import { backdropUrl, stillUrl } from "../lib/api";
import { fmtClock } from "../lib/format";
import type { ItemSummary, PlayInfo } from "../lib/types";

export function Row({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) {
  return (
    <section className="py-3">
      <div className="mb-3 flex items-baseline justify-between px-6">
        <h2 className="row-title">{title}</h2>
        {action}
      </div>
      <div className="row-scroll flex gap-5 overflow-x-auto px-6 pb-2 pt-1">{children}</div>
    </section>
  );
}

export function PosterRow({ title, items, action }: { title: string; items: ItemSummary[]; action?: ReactNode }) {
  if (!items.length) return null;
  return (
    <Row title={title} action={action}>
      {items.map((it) => (
        <div key={it.id} className="w-40 shrink-0">
          <PosterCard item={it} />
        </div>
      ))}
    </Row>
  );
}

/** 16:9 resume card: episode still (or the show's backdrop) with a progress strip. */
export function ContinueCard({ p }: { p: PlayInfo }) {
  const img = p.hasStill ? stillUrl(p.fileId) : p.hasBackdrop ? backdropUrl({ id: p.itemId, updatedAt: p.updatedAt }) : null;
  const left = p.durationSec ? Math.max(0, p.durationSec - p.positionSec) : 0;
  return (
    <Link to={`/play/${p.fileId}`} className="still-link group block w-72 shrink-0">
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
        <div className="art-progress">
          <span style={{ width: `${p.progress * 100}%` }} />
        </div>
      </div>
      <div className="poster-title">{p.title}</div>
      <div className="poster-meta">
        {[p.subtitle, left ? `${fmtClock(left)} left` : null].filter(Boolean).join(" · ")}
      </div>
    </Link>
  );
}
