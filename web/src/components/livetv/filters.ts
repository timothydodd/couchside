import { useEffect, useState } from "react";
import type { Program, TvChannel } from "../../lib/types";
import { useProfile } from "../../stores/profile";

export type QualityFilter = "all" | "hd" | "sd";

export interface TvFilters {
  q: string;
  genre: string; // "" = any
  newOnly: boolean;
  quality: QualityFilter;
  favoritesOnly: boolean;
  hideWeak: boolean;
  hideLocked: boolean;
}

export const DEFAULT_FILTERS: TvFilters = {
  q: "",
  genre: "",
  newOnly: false,
  quality: "all",
  favoritesOnly: false,
  hideWeak: false,
  hideLocked: false,
};

/** Signal quality below this is marginal: the picture may break up. */
export const WEAK_SIGNAL = 60;

/** Filter state shared by the Guide and Channels tabs, remembered per browser and profile. */
export function useTvFilters() {
  const KEY = `couchside:tv-filters:${useProfile((s) => s.current?.id ?? 0)}`;
  const [f, setF] = useState<TvFilters>(() => {
    try {
      return { ...DEFAULT_FILTERS, ...JSON.parse(localStorage.getItem(KEY) ?? "{}"), q: "" };
    } catch {
      return DEFAULT_FILTERS;
    }
  });
  useEffect(() => {
    try {
      const { q: _q, ...keep } = f; // don't remember the search text
      localStorage.setItem(KEY, JSON.stringify(keep));
    } catch {
      /* ignore */
    }
  }, [f, KEY]);
  return [f, setF] as const;
}

export const isFiltering = (f: TvFilters) =>
  !!f.q.trim() || !!f.genre || f.newOnly || f.quality !== "all" || f.favoritesOnly || f.hideWeak || f.hideLocked;

/** Filters that apply to programs (dim non-matching ones, hide channels with none). */
export const programFilterActive = (f: TvFilters) => !!f.q.trim() || !!f.genre || f.newOnly;

export function channelNameMatches(c: TvChannel, q: string): boolean {
  const n = q.trim().toLowerCase();
  return !!n && (c.name.toLowerCase().includes(n) || c.number.startsWith(n) || c.affiliate.toLowerCase().includes(n));
}

export function programMatches(p: Program, f: TvFilters): boolean {
  const n = f.q.trim().toLowerCase();
  if (n && !p.title.toLowerCase().includes(n) && !p.episodeTitle.toLowerCase().includes(n)) return false;
  if (f.genre && !p.categories.includes(f.genre)) return false;
  if (f.newOnly && !p.isNew) return false;
  return true;
}

/** Channel-level filters: quality, favorites, weak signal, locked. */
export function channelPasses(c: TvChannel, f: TvFilters): boolean {
  if (f.quality === "hd" && !c.hd) return false;
  if (f.quality === "sd" && c.hd) return false;
  if (f.favoritesOnly && !c.pinned) return false;
  if (f.hideWeak && c.signalQuality != null && c.signalQuality < WEAK_SIGNAL) return false;
  if (f.hideLocked && c.drm) return false;
  return true;
}

/** Genres present in a set of programs, most common first. */
export function genresOf(programs: Program[]): string[] {
  const count = new Map<string, number>();
  for (const p of programs) for (const c of p.categories) count.set(c, (count.get(c) ?? 0) + 1);
  return [...count.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).map(([g]) => g);
}
