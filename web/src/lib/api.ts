import { useCallback, useEffect, useRef, useState } from "react";

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
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
    try {
      const body = await res.json();
      if (body?.error) msg = body.error;
    } catch {
      /* not JSON */
    }
    throw new ApiError(res.status, msg);
  }
  if (res.status === 204 || res.headers.get("content-length") === "0") return undefined as T;
  // 202 Accepted may or may not carry a body (an optimize answers {queued}, a scan nothing).
  const text = await res.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

// Last response per URL, so navigating back paints instantly while it refreshes.
const cache = new Map<string, unknown>();

/**
 * Fetch JSON with stale-while-revalidate caching and optional polling. With
 * fresh, nothing is shown from the cache: data stays undefined until this
 * request answers (for values that mustn't be stale, like a resume point).
 */
export function useApi<T>(url: string | null, opts: { pollMs?: number; fresh?: boolean } = {}) {
  const cached = (u: string | null) => (u && !opts.fresh ? (cache.get(u) as T | undefined) : undefined);
  const [data, setData] = useState<T | undefined>(() => cached(url));
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(!!url && cached(url) === undefined);
  const urlRef = useRef(url);
  urlRef.current = url;

  const reload = useCallback(async () => {
    const u = urlRef.current;
    if (!u) return;
    try {
      const d = await api<T>(u);
      cache.set(u, d);
      if (urlRef.current === u) {
        setData(d);
        setError(null);
      }
    } catch (e) {
      if (urlRef.current === u) setError(e instanceof Error ? e.message : String(e));
    } finally {
      if (urlRef.current === u) setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (!url) return;
    const seed = cached(url);
    setData(seed);
    setLoading(seed === undefined);
    void reload();
    if (!opts.pollMs) return;
    const t = setInterval(() => {
      if (document.visibilityState === "visible") void reload();
    }, opts.pollMs);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [url, opts.pollMs, reload]);

  return { data, error, loading, reload };
}

export const posterUrl = (i: { id: number; updatedAt: number }, size: "thumb" | "full" = "thumb") =>
  `/api/artwork/items/${i.id}/${size === "thumb" ? "poster-thumb" : "poster"}?v=${i.updatedAt}`;
export const backdropUrl = (i: { id: number; updatedAt: number }) => `/api/artwork/items/${i.id}/backdrop?v=${i.updatedAt}`;
export const stillUrl = (fileId: number) => `/api/artwork/files/${fileId}/still`;
export const personPhotoUrl = (personId: number) => `/api/artwork/people/${personId}`;
export const streamUrl = (fileId: number) => `/api/files/${fileId}/stream`;
