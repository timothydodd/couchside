import { create } from "zustand";
import { api } from "../lib/api";
import { notify } from "../lib/notices";
import type { Prefs, Profile } from "../lib/types";

interface ProfileState {
  loaded: boolean;
  error: string | null;
  profiles: Profile[];
  /** The signed-in profile. */
  current: Profile | null;
  load: () => Promise<void>;
  /** Save preferences for the current profile (applied immediately). */
  setPrefs: (patch: Partial<Prefs>) => void;
}

export const useProfile = create<ProfileState>((set, get) => ({
  loaded: false,
  error: null,
  profiles: [],
  current: null,
  load: async () => {
    try {
      const r = await api<{ profiles: Profile[]; current: number }>("/api/profiles");
      const current = r.profiles.find((p) => p.id === r.current) ?? null;
      set({ loaded: true, error: null, profiles: r.profiles, current });
    } catch (e) {
      set({ loaded: true, error: e instanceof Error ? e.message : String(e) });
    }
  },
  setPrefs: (patch) => {
    const cur = get().current;
    if (!cur) return;
    const next = { ...cur, prefs: { ...cur.prefs, ...patch } };
    set({ current: next, profiles: get().profiles.map((p) => (p.id === cur.id ? next : p)) });
    api(`/api/profiles/${cur.id}/prefs`, { method: "PATCH", json: patch }).catch((e: unknown) => {
      // Not saved: put back what was there (unless it's been changed again
      // since), and say so, or the setting looks saved until the next reload.
      const now = get().current;
      if (now?.id === cur.id) {
        const prefs = { ...now.prefs } as Record<string, unknown>;
        const was = cur.prefs as Record<string, unknown>;
        for (const [k, v] of Object.entries(patch)) if (prefs[k] === v) prefs[k] = was[k];
        const back = { ...now, prefs: prefs as Prefs };
        set({ current: back, profiles: get().profiles.map((p) => (p.id === cur.id ? back : p)) });
      }
      notify(`Couldn't save that setting: ${e instanceof Error ? e.message : String(e)}`);
    });
  },
}));

/** The current profile's preferences. */
export const usePrefs = (): Prefs => useProfile((s) => s.current?.prefs) ?? NO_PREFS;
const NO_PREFS: Prefs = {};
