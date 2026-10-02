import { useCallback } from "react";
import { create } from "zustand";
import { ApiError, api, setAuthHooks } from "../lib/api";
import type { AuthInfo, Profile, SignedIn } from "../lib/types";

/**
 * Accounts are always on. Signing in is by name and password, or with
 * passwordless sign-in on (the default at home) by picking a profile. Tokens
 * live in HttpOnly cookies: a 15-minute access cookie, renewed in the
 * background before it runs out (video, images and hls.js use it too and
 * can't retry), and a refresh cookie per signed-in profile that only
 * /api/auth sees.
 */
interface AuthState {
  loaded: boolean;
  error: string | null;
  passwordless: boolean;
  passwordlessLocked: boolean;
  /** Passwordless: every profile; otherwise none. */
  profiles: AuthInfo["profiles"];
  setupRequired: boolean;
  user: Profile | null;
  signedIn: AuthInfo["signedIn"];
  /** Signing in here sends passwords unencrypted across the internet. */
  insecure: boolean;
  load: () => Promise<void>;
  login: (name: string, password: string) => Promise<void>;
  /** Passwordless sign-in to a profile without a password. */
  pick: (profileId: number) => Promise<void>;
  setup: (code: string, name: string, password: string) => Promise<void>;
  /** Switch to another profile this browser is signed in to. */
  switchTo: (profileId: number) => Promise<void>;
  logout: (everywhere?: boolean) => Promise<void>;
  changePassword: (current: string, password: string) => Promise<void>;
}

export const useAuth = create<AuthState>((set, get) => ({
  loaded: false,
  error: null,
  passwordless: false,
  passwordlessLocked: false,
  profiles: [],
  setupRequired: false,
  user: null,
  signedIn: [],
  insecure: false,
  load: async () => {
    try {
      const info = await api<AuthInfo>("/api/auth");
      set({
        passwordless: info.passwordless,
        passwordlessLocked: info.passwordlessLocked,
        profiles: info.profiles ?? [],
        setupRequired: info.setupRequired,
        user: info.user,
        signedIn: info.signedIn,
        insecure: !!info.insecure,
        error: null,
      });
      if (info.user && info.accessExpiresAt) schedule(info.accessExpiresAt);
      // The access cookie ran out while we were away; the refresh cookie may still be good.
      else if (!info.user && info.signedIn.length) await refreshSession();
    } catch (e) {
      set({ error: e instanceof Error ? e.message : String(e) });
    }
    set({ loaded: true });
  },
  login: async (name, password) => {
    await api<SignedIn>("/api/auth/login", { method: "POST", json: { name, password, client: "web" } });
    location.assign("/");
  },
  pick: async (profileId) => {
    await api<SignedIn>("/api/auth/pick", { method: "POST", json: { profileId, client: "web" } });
    location.assign("/");
  },
  setup: async (code, name, password) => {
    await api<SignedIn>("/api/auth/setup", { method: "POST", json: { code, name, password } });
    location.assign("/");
  },
  switchTo: async (profileId) => {
    if (!(await refreshSession(profileId))) {
      await get().load();
      throw new Error("That sign-in has ended; enter the password again.");
    }
    // Everything on screen belongs to the old profile.
    location.assign("/");
  },
  logout: async (everywhere = false) => {
    await api("/api/auth/logout", { method: "POST", json: { everywhere } });
    location.assign("/");
  },
  changePassword: async (current, password) => {
    await api("/api/auth/password", { method: "POST", json: { current, password } });
    const u = get().user;
    if (u?.mustChangePassword) set({ user: { ...u, mustChangePassword: false } });
  },
}));

// --- keeping the access token fresh ------------------------------------------------

let refreshing: Promise<boolean> | null = null;
let timer: ReturnType<typeof setTimeout> | undefined;
let expiresAt = 0;

/**
 * Trades the refresh cookie for new tokens; one at a time. A 409 means another
 * tab just did it and the browser already has the new cookie, so try again.
 */
export function refreshSession(profileId?: number): Promise<boolean> {
  if (refreshing && !profileId) return refreshing;
  const run = async () => {
    for (let attempt = 0; attempt < 3; attempt++) {
      const res = await fetch("/api/auth/refresh", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(profileId ? { profileId } : {}),
      }).catch(() => null);
      if (!res) return false;
      if (res.ok) {
        const t = (await res.json()) as SignedIn;
        useAuth.setState({ user: t.user });
        schedule(t.accessExpiresAt);
        return true;
      }
      if (res.status !== 409) return false;
      await new Promise((r) => setTimeout(r, 300 + Math.random() * 400));
    }
    return false;
  };
  refreshing = run().finally(() => {
    refreshing = null;
  });
  return refreshing;
}

/** Renew two minutes before the access token runs out. */
function schedule(exp: number) {
  expiresAt = exp;
  clearTimeout(timer);
  const ms = exp * 1000 - Date.now() - 120_000;
  timer = setTimeout(() => void refreshSession(), Math.max(5_000, ms));
}

// Timers stall in background tabs and sleeping laptops; catch up on return.
document.addEventListener("visibilitychange", () => {
  const { user } = useAuth.getState();
  if (document.visibilityState === "visible" && user && expiresAt * 1000 - Date.now() < 180_000) void refreshSession();
});

setAuthHooks({
  refresh: async () => {
    const ok = await refreshSession();
    // The session is over (signed out elsewhere, expired): back to sign-in.
    if (!ok) useAuth.setState({ user: null });
    return ok;
  },
  passwordChange: () => {
    const u = useAuth.getState().user;
    if (u && !u.mustChangePassword) useAuth.setState({ user: { ...u, mustChangePassword: true } });
  },
});

/** Admins can reach settings, libraries and management (everyone, with accounts off). */
export const useIsAdmin = () => useAuth((s) => s.user?.role === "admin");
/** Admins, and users an admin has allowed, can schedule recordings. */
export const useCanRecord = () => useAuth((s) => s.user?.role === "admin" || !!s.user?.canRecord);
/** Checks whether the user may change a recording or rule by its ownerId (0 = admins only). */
export function useOwnerCheck() {
  const admin = useIsAdmin();
  const me = useAuth((s) => s.user?.id);
  return useCallback((ownerId: number) => admin || (ownerId !== 0 && ownerId === me), [admin, me]);
}

/** A readable message for a failed sign-in call. */
export function authError(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  return e instanceof Error ? e.message : String(e);
}
