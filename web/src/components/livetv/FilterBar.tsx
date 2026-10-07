import { Lock, SignalLow, Sparkles, Star } from "lucide-react";
import type { ReactNode } from "react";
import FilterMenu, { ActiveChip, Choices } from "../FilterMenu";
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
  // Everything but the search box lives in the menu; what's on shows as chips.
  const on: { label: string; clear: Partial<TvFilters> }[] = [
    ...(f.genre ? [{ label: f.genre, clear: { genre: "" } }] : []),
    ...(f.quality !== "all" ? [{ label: f.quality === "hd" ? "HD only" : "SD only", clear: { quality: "all" as QualityFilter } }] : []),
    ...(f.newOnly ? [{ label: "New", clear: { newOnly: false } }] : []),
    ...(f.favoritesOnly ? [{ label: "Favorites", clear: { favoritesOnly: false } }] : []),
    ...(f.hideWeak ? [{ label: "Hide weak", clear: { hideWeak: false } }] : []),
    ...(f.hideLocked ? [{ label: "Hide locked", clear: { hideLocked: false } }] : []),
  ];
  return (
    <div className="gutter py-3">
      {children && <div className="mb-3 flex flex-wrap items-center gap-2">{children}</div>}
      <div className="flex items-center gap-2">
        <SearchInput large value={f.q} onChange={(q) => patch({ q })} placeholder="Search channels and shows" aria-label="Search channels and shows" className="min-w-0 flex-1 md:max-w-xl" />
        <FilterMenu active={on.length} onReset={() => set({ ...DEFAULT_FILTERS, q: f.q })}>
          {genres.length > 0 && (
            <Choices label="Genre" value={f.genre} onChange={(genre) => patch({ genre })} options={[{ id: "", label: "All genres" }, ...genres.map((g) => ({ id: g, label: g }))]} />
          )}
          <Choices<QualityFilter>
            label="Quality"
            value={f.quality}
            onChange={(quality) => patch({ quality })}
            options={[
              { id: "all", label: "HD & SD" },
              { id: "hd", label: "HD only" },
              { id: "sd", label: "SD only" },
            ]}
          />
          <div>
            <div className="field-label">Only</div>
            <div className="flex flex-wrap gap-1.5">
              <Toggle on={f.newOnly} onClick={() => patch({ newOnly: !f.newOnly })} title="Only first airings">
                <Sparkles size={13} /> New
              </Toggle>
              <Toggle on={f.favoritesOnly} onClick={() => patch({ favoritesOnly: !f.favoritesOnly })} title="Only pinned channels">
                <Star size={13} className={f.favoritesOnly ? "fill-current" : ""} /> Favorites
              </Toggle>
            </div>
          </div>
          <div>
            <div className="field-label">Hide</div>
            <div className="flex flex-wrap gap-1.5">
              <Toggle on={f.hideWeak} onClick={() => patch({ hideWeak: !f.hideWeak })} title="Hide channels with marginal signal (quality under 60%)">
                <SignalLow size={13} /> Weak signal
              </Toggle>
              <Toggle on={f.hideLocked} onClick={() => patch({ hideLocked: !f.hideLocked })} title="Hide copy-protected ATSC 3.0 channels">
                <Lock size={13} /> Locked
              </Toggle>
            </div>
          </div>
        </FilterMenu>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-1.5">
        {on.map((c) => (
          <ActiveChip key={c.label} label={c.label} onClear={() => patch(c.clear)} />
        ))}
        <span className="ml-auto text-xs text-content-muted">{isFiltering(f) ? `${shown} of ${total} channels` : `${total} channels`}</span>
      </div>
    </div>
  );
}

/** An on/off filter chip. */
function Toggle({ on, onClick, title, children }: { on: boolean; onClick: () => void; title: string; children: ReactNode }) {
  return (
    <button type="button" onClick={onClick} title={title} aria-pressed={on} className={`choice inline-flex items-center gap-1.5 ${on ? "choice-on" : ""}`}>
      {children}
    </button>
  );
}
