import { create } from "zustand";

/** Settings pages: "you" (/settings) is everyone's own; the rest are admin-only. */
export const SETTINGS_SECTIONS = ["you", "system", "console", "accounts", "metadata", "livetv", "advanced"] as const;
export type SettingsSection = (typeof SETTINGS_SECTIONS)[number];

export type Route =
  | { name: "home" }
  | { name: "movies" }
  | { name: "tv" }
  | { name: "item"; id: number }
  | { name: "play"; fileId: number }
  | { name: "livetv"; tab: "guide" | "channels" | "recordings" }
  | { name: "watch"; channel: string }
  | { name: "recording"; id: number }
  | { name: "activity" }
  | { name: "libraries" }
  | { name: "manage"; id: number }
  | { name: "settings"; section: SettingsSection }
  | { name: "profiles" }
  | { name: "admin" }
  | { name: "link" }
  | { name: "search"; q: string }
  | { name: "person"; id: number }
  | { name: "notfound" };

/** Parses a path, with its query string if it has one. */
export function parseRoute(path: string): Route {
  const [pathname, query = ""] = path.split("?", 2);
  const p = pathname.replace(/\/+$/, "") || "/";
  if (p === "/search") return { name: "search", q: new URLSearchParams(query).get("q") ?? "" };
  if (p === "/") return { name: "home" };
  if (p === "/movies") return { name: "movies" };
  if (p === "/tv") return { name: "tv" };
  if (p === "/activity") return { name: "activity" };
  if (p === "/livetv") return { name: "livetv", tab: "guide" };
  if (p === "/livetv/channels") return { name: "livetv", tab: "channels" };
  if (p === "/livetv/recordings") return { name: "livetv", tab: "recordings" };
  if (p === "/libraries") return { name: "libraries" };
  if (p === "/settings") return { name: "settings", section: "you" };
  if (p === "/profiles") return { name: "profiles" };
  if (p === "/admin") return { name: "admin" };
  if (p === "/link") return { name: "link" };
  let m = p.match(/^\/settings\/([a-z]+)$/);
  if (m && (SETTINGS_SECTIONS as readonly string[]).includes(m[1]) && m[1] !== "you") return { name: "settings", section: m[1] as SettingsSection };
  m = p.match(/^\/item\/(\d+)$/);
  if (m) return { name: "item", id: Number(m[1]) };
  m = p.match(/^\/person\/(\d+)$/);
  if (m) return { name: "person", id: Number(m[1]) };
  m = p.match(/^\/libraries\/(\d+)$/);
  if (m) return { name: "manage", id: Number(m[1]) };
  m = p.match(/^\/recording\/(\d+)$/);
  if (m) return { name: "recording", id: Number(m[1]) };
  m = p.match(/^\/watch\/([\d.]+)$/);
  if (m) return { name: "watch", channel: m[1] };
  m = p.match(/^\/play\/(\d+)$/);
  if (m) return { name: "play", fileId: Number(m[1]) };
  return { name: "notfound" };
}

interface RouterState {
  path: string;
  route: Route;
  go: (path: string, opts?: { replace?: boolean }) => void;
  back: (fallback: string) => void;
}

/** Tiny history-API router: a handful of routes doesn't justify react-router. */
export const useRouter = create<RouterState>((set, get) => ({
  path: here(),
  route: parseRoute(here()),
  go: (path, opts) => {
    if (path === get().path) return;
    if (opts?.replace) history.replaceState({ couchside: true }, "", path);
    else history.pushState({ couchside: true }, "", path);
    set({ path, route: parseRoute(path) });
  },
  back: (fallback) => {
    // Only go back if the previous entry is ours; otherwise go to the fallback.
    if (history.state?.couchside) history.back();
    else get().go(fallback, { replace: true });
  },
}));

function here() {
  return location.pathname + location.search;
}

window.addEventListener("popstate", () => {
  useRouter.setState({ path: here(), route: parseRoute(here()) });
});
