import { useCallback } from "react";
import { create } from "zustand";
import { ApiError, api, setAuthHooks } from "../lib/api";
import type { AuthInfo, Profile, SignedIn } from "../lib/types";
import { stopStatus } from "./status";
import { errText } from "../lib/errors";

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
  /** Passwordless: the profiles to pick from; otherwise none. At /admin while admins are hidden, the admins. */
  profiles: AuthInfo["profiles"];
  /** Admin accounts are left off the picker and sign in at /admin. */
  hideAdmins: boolean;
  setupRequired: boolean;
  /** The first-run setup (your name, your media) isn't done. */
  firstRun: boolean;
  /** First run from outside the home network: welcome needs the setup code and a password. */
  remoteFirstRun: boolean;
  user: Profile | null;
  signedIn: AuthInfo["signedIn"];
  /** Signing in here sends passwords unencrypted across the internet. */
  insecure: boolean;
  /** The label of the single sign-on button; "" when there's none. */
  oidc: string;
  load: () => Promise<void>;
  /** code is the second step, for a profile with two-step sign-in; without it the error's code is "totp_required". */
  login: (name: string, password: string, code?: string) => Promise<void>;
  /** Passwordless sign-in to a profile without a password. */
  pick: (profileId: number) => Promise<void>;
  setup: (code: string, name: string, password: string) => Promise<void>;
  /** First run, passwordless: name the admin profile (and maybe give it a password) and sign in. From outside the home network it takes the setup code. */
  welcome: (name: string, password: string, code?: string) => Promise<void>;
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
  hideAdmins: false,
  setupRequired: false,
  firstRun: false,
  remoteFirstRun: false,
  user: null,
  signedIn: [],
  insecure: false,
  oidc: "",
  load: async () => {
    try {
      // /admin is where hidden admin accounts sign in: ask for them there.
      const admin = location.pathname.replace(/\/+$/, "") === "/admin";
      const info = await api<AuthInfo>(admin ? "/api/auth?admin=1" : "/api/auth");
      set({
        passwordless: info.passwordless,
        passwordlessLocked: info.passwordlessLocked,
        profiles: info.profiles ?? [],
        hideAdmins: !!info.hideAdmins,
        setupRequired: info.setupRequired,
        firstRun: !!info.firstRun,
        remoteFirstRun: !!info.remoteFirstRun,
        user: info.user,
        signedIn: info.signedIn,
        insecure: !!info.insecure,
        oidc: info.oidc ?? "",
        error: null,
      });
      if (info.user && info.accessExpiresAt) schedule(info);
      // The access cookie ran out while we were away; the refresh cookie may still be good.
      else if (!info.user && info.signedIn.length) await refreshSession();
    } catch (e) {
      set({ error: errText(e) });
    }
    set({ loaded: true });
  },
  login: async (name, password, code) => {
    await api<SignedIn>("/api/auth/login", { method: "POST", json: { name, password, code, client: "web" } });
    announceProfileChange();
    location.assign("/");
  },
  pick: async (profileId) => {
    await api<SignedIn>("/api/auth/pick", { method: "POST", json: { profileId, client: "web" } });
    announceProfileChange();
    location.assign("/");
  },
  setup: async (code, name, password) => {
    await api<SignedIn>("/api/auth/setup", { method: "POST", json: { code, name, password } });
    announceProfileChange();
    location.assign("/");
  },
  welcome: async (name, password, code) => {
    await api<SignedIn>("/api/auth/welcome", { method: "POST", json: { name, password, code } });
    announceProfileChange();
    location.assign("/");
  },
  switchTo: async (profileId) => {
    if (!(await refreshSession(profileId))) {
      await get().load();
      throw new Error("That sign-in has ended; enter the password again.");
    }
    // Everything on screen belongs to the old profile, here and in other tabs.
    announceProfileChange();
    location.assign("/");
  },
  logout: async (everywhere = false) => {
    await api("/api/auth/logout", { method: "POST", json: { everywhere } });
    announceProfileChange();
    location.assign("/");
  },
  changePassword: async (current, password) => {
    await api("/api/auth/password", { method: "POST", json: { current, password } });
    const u = get().user;
    if (u?.mustChangePassword) set({ user: { ...u, mustChangePassword: false } });
  },
}));

// --- other tabs ---------------------------------------------------------------------

// Cookies are shared by every tab, so after a sign-in, sign-out or profile
// switch here, other tabs would carry on as the new profile (a player writing
// its progress into someone else's history). Tell them to reload.
const tabs = typeof BroadcastChannel === "undefined" ? null : new BroadcastChannel("couchside-profile");
tabs?.addEventListener("message", () => location.reload());
function announceProfileChange() {
  tabs?.postMessage("changed");
}

// Nobody's signed in: stop asking the server for status.
useAuth.subscribe((s, prev) => {
  if (prev.user && !s.user) stopStatus();
});

// --- keeping the access token fresh ------------------------------------------------

/** ok: renewed. ended: the session is over (401). failed: the network or server let us down; the session may be fine. */
type RefreshResult = "ok" | "ended" | "failed";

let refreshing: Promise<RefreshResult> | null = null;
let timer: ReturnType<typeof setTimeout> | undefined;
let expiresAt = 0;

/**
 * Trades the refresh cookie for new tokens; one at a time. A 409 means another
 * tab just did it and the browser already has the new cookie, so try again.
 * Network errors and server errors are retried, and then reported as failed,
 * never as signed out: only the server saying 401 ends the session here.
 */
function refresh(profileId?: number): Promise<RefreshResult> {
  if (refreshing && !profileId) return refreshing;
  const run = async (): Promise<RefreshResult> => {
    for (let attempt = 0; attempt < 4; attempt++) {
      if (attempt) await new Promise((r) => setTimeout(r, attempt * 700 + Math.random() * 400));
      const res = await fetch("/api/auth/refresh", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(profileId ? { profileId } : {}),
      }).catch(() => null);
      if (res?.ok) {
        const t = (await res.json()) as SignedIn;
        useAuth.setState({ user: t.user });
        schedule(t);
        return "ok";
      }
      if (res?.status === 401) return "ended";
      if (res && res.status !== 409 && res.status < 500) return "failed";
    }
    return "failed";
  };
  const p = run().then((r) => {
    // Try again in a while rather than leave the access token to run out.
    if (r === "failed" && !profileId) {
      clearTimeout(timer);
      timer = setTimeout(() => void refresh(), 30_000);
    }
    return r;
  });
  refreshing = p.finally(() => {
    refreshing = null;
  });
  return refreshing;
}

/** Renews the session, or switches to profileId's; true when it worked. */
export async function refreshSession(profileId?: number): Promise<boolean> {
  return (await refresh(profileId)) === "ok";
}

/**
 * Renew two minutes before the access token runs out. The time left comes
 * from the server (expiresIn), so a browser clock that's minutes fast or
 * slow doesn't renew every few seconds, or after the cookie has gone.
 * expiresAt is kept on this browser's clock.
 */
function schedule(t: { expiresIn?: number; accessExpiresAt?: number }) {
  const left = t.expiresIn ?? (t.accessExpiresAt ?? 0) - Date.now() / 1000;
  expiresAt = Date.now() / 1000 + left;
  clearTimeout(timer);
  timer = setTimeout(() => void refresh(), Math.max(5_000, left * 1000 - 120_000));
}

// Timers stall in background tabs and sleeping laptops; catch up on return.
document.addEventListener("visibilitychange", () => {
  const { user } = useAuth.getState();
  if (document.visibilityState === "visible" && user && expiresAt * 1000 - Date.now() < 180_000) void refresh();
});

setAuthHooks({
  refresh: async () => {
    const r = await refresh();
    // The session is over (signed out elsewhere, expired): back to sign-in.
    // A failed attempt keeps the user; the request fails and is retried later.
    if (r === "ended") useAuth.setState({ user: null });
    return r === "ok";
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
  return errText(e);
}
