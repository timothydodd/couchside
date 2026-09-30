import { useState } from "react";
import { Check, Star } from "lucide-react";
import Link from "./Link";
import { posterUrl } from "../lib/api";
import { placeholderAngle } from "../lib/format";
import type { ItemSummary } from "../lib/types";

/** Poster art, or a branded gradient with the title when there's no poster (bare: no title, for tiny thumbnails). */
export function PosterArt({
  item,
  size = "thumb",
  bare = false,
}: {
  item: Pick<ItemSummary, "id" | "hasPoster" | "updatedAt" | "title" | "year">;
  size?: "thumb" | "full";
  bare?: boolean;
}) {
  const [failed, setFailed] = useState(false);
  if (item.hasPoster && !failed) {
    return (
      <img
        src={posterUrl(item, size)}
        alt=""
        loading="lazy"
        decoding="async"
        onError={() => setFailed(true)}
        className="absolute inset-0 h-full w-full object-cover"
      />
    );
  }
  return (
    <div
      aria-hidden="true"
      className="poster-placeholder absolute inset-0 flex flex-col justify-end p-3"
      style={{ "--ph-angle": placeholderAngle(item.id) } as React.CSSProperties}
    >
      {!bare && (
        <>
          <div className="line-clamp-4 text-base font-semibold leading-snug text-content">{item.title}</div>
          {item.year && <div className="mt-1 text-xs text-content-muted">{item.year}</div>}
        </>
      )}
    </div>
  );
}

export default function PosterCard({ item }: { item: ItemSummary }) {
  const watched = item.fileCount > 0 && item.watchedCount >= item.fileCount;
  const partial = item.kind === "series" && item.watchedCount > 0 && !watched;
  const meta = [
    item.year,
    item.kind === "series" ? `${item.fileCount} ep${item.fileCount === 1 ? "" : "s"}` : null,
  ].filter(Boolean);

  return (
    <Link to={`/item/${item.id}`} className="poster-link group block" title={item.title}>
      <div className="poster">
        <PosterArt item={item} />
        <div className="absolute right-1.5 top-1.5 flex flex-col items-end gap-1">
          {watched && (
            <span className="art-badge tint-good !bg-good/90 !text-on-accent" title="Watched">
              <Check size={12} strokeWidth={3} />
            </span>
          )}
          {item.matchStatus === "unmatched" && <span className="art-badge scrim">Unmatched</span>}
        </div>
        {item.rating != null && (
          <span className="art-badge scrim absolute bottom-1.5 left-1.5 opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100">
            <Star size={11} className="fill-warning text-warning" />
            {item.rating.toFixed(1)}
          </span>
        )}
        {partial && (
          <div className="art-progress">
            <span style={{ width: `${(item.watchedCount / item.fileCount) * 100}%` }} />
          </div>
        )}
      </div>
      <div className="poster-title">{item.title}</div>
      <div className="poster-meta">{meta.join(" · ") || " "}</div>
    </Link>
  );
}
