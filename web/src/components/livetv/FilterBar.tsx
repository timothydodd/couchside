import { Lock, SignalLow, Sparkles, Star, X } from "lucide-react";
import type { ReactNode } from "react";
import { SearchInput } from "../ui";
import { DEFAULT_FILTERS, isFiltering, type QualityFilter, type TvFilters } from "./filters";

/** Search and filter controls for the Live TV guide and channel list. */
export default function FilterBar({
  f,
  set,
  genres,
  shown,
  total,
  children,
}: {
  f: TvFilters;
  set: (f: TvFilters) => void;
  genres: string[];
  shown: number;
  total: number;
  children?: ReactNode; // extra controls (guide time navigation)
}) {
  const patch = (p: Partial<TvFilters>) => set({ ...f, ...p });
  return (
    <div className="flex flex-wrap items-center gap-2 px-6 py-3">
      {children}
      <SearchInput value={f.q} onChange={(q) => patch({ q })} placeholder="Channel or show…" className="w-56" />
      <select className="field" value={f.genre} onChange={(e) => patch({ genre: e.target.value })} aria-label="Genre">
        <option value="">All genres</option>
        {genres.map((g) => (
          <option key={g}>{g}</option>
        ))}
      </select>
      <select className="field" value={f.quality} onChange={(e) => patch({ quality: e.target.value as QualityFilter })} aria-label="Stream quality">
        <option value="all">HD &amp; SD</option>
        <option value="hd">HD only</option>
        <option value="sd">SD only</option>
      </select>
      <Toggle on={f.newOnly} onClick={() => patch({ newOnly: !f.newOnly })} title="Only first airings">
        <Sparkles size={13} /> New
      </Toggle>
      <Toggle on={f.favoritesOnly} onClick={() => patch({ favoritesOnly: !f.favoritesOnly })} title="Only pinned channels">
        <Star size={13} className={f.favoritesOnly ? "fill-current" : ""} /> Favorites
      </Toggle>
      <Toggle on={f.hideWeak} onClick={() => patch({ hideWeak: !f.hideWeak })} title="Hide channels with marginal signal (quality under 60%)">
        <SignalLow size={13} /> Hide weak
      </Toggle>
      <Toggle on={f.hideLocked} onClick={() => patch({ hideLocked: !f.hideLocked })} title="Hide copy-protected ATSC 3.0 channels">
        <Lock size={13} /> Hide locked
      </Toggle>
      <span className="ml-auto flex items-center gap-2 text-xs text-content-muted">
        {isFiltering(f) ? `${shown} of ${total} channels` : `${total} channels`}
        {isFiltering(f) && (
          <button className="btn-quiet !px-1.5 !py-0.5 !text-xs" onClick={() => set(DEFAULT_FILTERS)}>
            <X size={12} /> Clear
          </button>
        )}
      </span>
    </div>
  );
}

function Toggle({ on, onClick, title, children }: { on: boolean; onClick: () => void; title: string; children: ReactNode }) {
  return (
    <button
      onClick={onClick}
      title={title}
      aria-pressed={on}
      className={`inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1.5 text-xs font-medium transition-colors ${
        on ? "border-accent bg-accent/15 text-content" : "border-border text-content-secondary hover:border-accent hover:text-content"
      }`}
    >
      {children}
    </button>
  );
}
