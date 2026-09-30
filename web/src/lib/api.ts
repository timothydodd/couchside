import { useCallback, useEffect, useRef, useState } from "react";

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

export async function api<T = void>(path: string, init?: RequestInit & { json?: unknown }): Promise<T> {
  const { json, ...rest } = init ?? {};
  const res = await fetch(path, {
    ...rest,
    headers: json !== undefined ? { "Content-Type": "application/json", ...rest.headers } : rest.headers,
    body: json !== undefined ? JSON.stringify(json) : rest.body,
  });
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
  if (res.status === 204 || res.status === 202 || res.headers.get("content-length") === "0") return undefined as T;
  return (await res.json()) as T;
}

// Last response per URL, so navigating back paints instantly while it refreshes.
const cache = new Map<string, unknown>();

/** Fetch JSON with stale-while-revalidate caching and optional polling. */
export function useApi<T>(url: string | null, opts: { pollMs?: number } = {}) {
  const [data, setData] = useState<T | undefined>(() => (url ? (cache.get(url) as T | undefined) : undefined));
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(!!url && !cache.has(url));
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
    setData(cache.get(url) as T | undefined);
    setLoading(!cache.has(url));
    void reload();
    if (!opts.pollMs) return;
    const t = setInterval(() => {
      if (document.visibilityState === "visible") void reload();
    }, opts.pollMs);
    return () => clearInterval(t);
  }, [url, opts.pollMs, reload]);

  return { data, error, loading, reload };
}

export const posterUrl = (i: { id: number; updatedAt: number }, size: "thumb" | "full" = "thumb") =>
  `/api/artwork/items/${i.id}/${size === "thumb" ? "poster-thumb" : "poster"}?v=${i.updatedAt}`;
export const backdropUrl = (i: { id: number; updatedAt: number }) => `/api/artwork/items/${i.id}/backdrop?v=${i.updatedAt}`;
export const stillUrl = (fileId: number) => `/api/artwork/files/${fileId}/still`;
export const streamUrl = (fileId: number) => `/api/files/${fileId}/stream`;
