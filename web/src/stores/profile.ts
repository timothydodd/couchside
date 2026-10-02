import { create } from "zustand";
import { api } from "../lib/api";
import type { Prefs, Profile, ProfileColor } from "../lib/types";

interface ProfileState {
  loaded: boolean;
  error: string | null;
  profiles: Profile[];
  current: Profile | null;
  /** This browser picked a profile; otherwise the server fell back to the first one. */
  chosen: boolean;
  load: () => Promise<void>;
  /** Switch profile. Everything on screen belongs to the old one, so the app reloads. */
  select: (id: number) => Promise<void>;
  create: (name: string, color: ProfileColor) => Promise<Profile>;
  update: (id: number, name: string, color: ProfileColor) => Promise<void>;
  remove: (id: number) => Promise<void>;
  /** Save preferences for the current profile (applied immediately). */
  setPrefs: (patch: Partial<Prefs>) => void;
}

export const useProfile = create<ProfileState>((set, get) => ({
  loaded: false,
  error: null,
  profiles: [],
  current: null,
  chosen: false,
  load: async () => {
    try {
      const r = await api<{ profiles: Profile[]; current: number; chosen: boolean }>("/api/profiles");
      const current = r.profiles.find((p) => p.id === r.current) ?? null;
      set({ loaded: true, error: null, profiles: r.profiles, current, chosen: r.chosen });
    } catch (e) {
      set({ loaded: true, error: e instanceof Error ? e.message : String(e) });
    }
  },
  select: async (id) => {
    await api(`/api/profiles/${id}/select`, { method: "POST" });
    location.assign("/");
  },
  create: async (name, color) => {
    const p = await api<Profile>("/api/profiles", { method: "POST", json: { name, color } });
    await get().load();
    return p;
  },
  update: async (id, name, color) => {
    await api(`/api/profiles/${id}`, { method: "PUT", json: { name, color } });
    await get().load();
  },
  remove: async (id) => {
    await api(`/api/profiles/${id}`, { method: "DELETE" });
    if (id === get().current?.id) return void location.assign("/profiles");
    await get().load();
  },
  setPrefs: (patch) => {
    const cur = get().current;
    if (!cur) return;
    const next = { ...cur, prefs: { ...cur.prefs, ...patch } };
    set({ current: next, profiles: get().profiles.map((p) => (p.id === cur.id ? next : p)) });
    void api(`/api/profiles/${cur.id}/prefs`, { method: "PATCH", json: patch }).catch(() => {});
  },
}));


/** The current profile's preferences. */
export const usePrefs = (): Prefs => useProfile((s) => s.current?.prefs) ?? NO_PREFS;
const NO_PREFS: Prefs = {};
