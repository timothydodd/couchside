import { useCallback, useEffect, useRef, useState } from "react";
import { errText } from "./errors";

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    /** The server's machine-readable reason, when it gives one ("totp_required"). */
    public code?: string,
  ) {
    super(message);
  }
}

/**
 * Set by the auth store when accounts are on: refresh renews an expired
 * access token (false when the session is over), and passwordChange is told
 * when the server wants a temporary password replaced first.
 */
interface AuthHooks {
  refresh: () => Promise<boolean>;
  passwordChange: () => void;
}
let authHooks: AuthHooks | null = null;
export const setAuthHooks = (h: AuthHooks) => {
  authHooks = h;
};

async function send(path: string, init: RequestInit & { json?: unknown }, retried = false): Promise<Response> {
  const { json, ...rest } = init;
  const res = await fetch(path, {
    ...rest,
    headers: json !== undefined ? { "Content-Type": "application/json", ...rest.headers } : rest.headers,
    body: json !== undefined ? JSON.stringify(json) : rest.body,
  });
  // An expired access token: renew it once and try again.
  if (res.status === 401 && authHooks && !retried && !path.startsWith("/api/auth")) {
    if (await authHooks.refresh()) return send(path, init, true);
  }
  if (res.status === 403 && authHooks) {
    const body = await res
      .clone()
      .json()
      .catch(() => null);
    if (body?.code === "password_change_required") authHooks.passwordChange();
  }
  return res;
}

export async function api<T = void>(path: string, init?: RequestInit & { json?: unknown }): Promise<T> {
  const res = await send(path, init ?? {});
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`;
    let code: string | undefined;
    try {
      const body = await res.json();
      if (body?.error) msg = body.error;
      if (typeof body?.code === "string") code = body.code;
    } catch {
      /* not JSON */
    }
    throw new ApiError(res.status, msg, code);
  }
  if (res.status === 204 || res.headers.get("content-length") === "0") return undefined as T;
  // 202 Accepted may or may not carry a body (an optimize answers {queued}, a scan nothing).
  const text = await res.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

// Last response per URL, so navigating back paints instantly while it
// refreshes. Capped, least recently used first: search alone makes a URL per
// keystroke, and a tab can stay open for weeks.
const CACHE_MAX = 300;
const cache = new Map<string, unknown>();

function remember(url: string, data: unknown) {
  cache.delete(url);
  cache.set(url, data);
  if (cache.size > CACHE_MAX) cache.delete(cache.keys().next().value as string);
}

function recall(url: string): unknown {
  const data = cache.get(url);
  if (data !== undefined) remember(url, data); // used again: keep it longest
  return data;
}

interface ApiState<T> {
  url: string | null; // what data and error belong to
  data: T | undefined;
  error: string | null;
  loading: boolean;
}

/**
 * Fetch JSON with stale-while-revalidate caching and optional polling. With
 * fresh, nothing is shown from the cache: data stays undefined until this
 * request answers (for values that mustn't be stale, like a resume point).
 *
 * What it returns always belongs to the url passed in: after the url changes
 * it's that url's cached answer (or nothing) and no error, never the last
 * url's. An answer is dropped when a newer request for the same url has been
 * sent since, so a slow poll can't overwrite the reload after a change.
 *
 * keep is for views that page through one list by changing the url (the
 * guide's hours, a folder picker, a chart's range): the last url's data
 * stays up, with loading set, until the new url answers, so the view doesn't
 * blank between pages.
 */
export function useApi<T>(url: string | null, opts: { pollMs?: number; fresh?: boolean; keep?: boolean } = {}) {
  const seed = (u: string | null): ApiState<T> => {
    const data = u && !opts.fresh ? (recall(u) as T | undefined) : undefined;
    return { url: u, data, error: null, loading: !!u && data === undefined };
  };
  const move = (prev: ApiState<T>, u: string | null): ApiState<T> => {
    const s = seed(u);
    return opts.keep && u && s.data === undefined ? { ...s, data: prev.data } : s;
  };
  const [state, setState] = useState<ApiState<T>>(() => seed(url));
  const urlRef = useRef(url);
  urlRef.current = url;
  const sent = useRef(0); // requests sent so far; an answer counts only if it's the latest

  const reload = useCallback(async () => {
    const u = urlRef.current;
    if (!u) return;
    const mine = ++sent.current;
    const latest = () => urlRef.current === u && sent.current === mine;
    try {
      const d = await api<T>(u);
      remember(u, d);
      if (latest()) setState({ url: u, data: d, error: null, loading: false });
    } catch (e) {
      const error = errText(e);
      if (latest()) setState((s) => ({ url: u, data: s.url === u ? s.data : undefined, error, loading: false }));
    }
  }, []);

  useEffect(() => {
    setState((prev) => (prev.url === url ? prev : move(prev, url)));
    if (!url) return;
    void reload();
    if (!opts.pollMs) return;
    const t = setInterval(() => {
      if (document.visibilityState === "visible") void reload();
    }, opts.pollMs);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [url, opts.pollMs, reload]);

  // The effect above runs after this render: until then, state is the last url's.
  const cur = state.url === url ? state : move(state, url);
  return { data: cur.data, error: cur.error, loading: cur.loading, reload };
}

export const posterUrl = (i: { id: number; updatedAt: number }, size: "thumb" | "full" = "thumb") =>
  `/api/artwork/items/${i.id}/${size === "thumb" ? "poster-thumb" : "poster"}?v=${i.updatedAt}`;
export const backdropUrl = (i: { id: number; updatedAt: number }) => `/api/artwork/items/${i.id}/backdrop?v=${i.updatedAt}`;
export const stillUrl = (fileId: number) => `/api/artwork/files/${fileId}/still`;
export const personPhotoUrl = (personId: number) => `/api/artwork/people/${personId}`;
