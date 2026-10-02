import { Activity, Clapperboard, FolderOpen, Home, Moon, RadioTower, Settings, Sun, Tv, type LucideIcon } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import Link from "./Link";
import ProfileAvatar from "./ProfileAvatar";
import { SearchInput } from "./ui";
import { useRouter, type Route } from "../stores/router";
import { useStatus } from "../stores/status";
import { useIsAdmin } from "../stores/auth";
import { setTheme, useProfile } from "../stores/profile";
import { useThemeStore } from "../stores/theme";

type NavName = Route["name"];

const ITEMS: { name: NavName; to: string; label: string; Icon: LucideIcon }[] = [
  { name: "home", to: "/", label: "Home", Icon: Home },
  { name: "movies", to: "/movies", label: "Movies", Icon: Clapperboard },
  { name: "tv", to: "/tv", label: "TV Shows", Icon: Tv },
];

const LIVE = { name: "livetv" as NavName, to: "/livetv", label: "Live TV", Icon: RadioTower };

const MANAGE: { name: NavName; to: string; label: string; Icon: LucideIcon }[] = [
  { name: "activity", to: "/activity", label: "Activity", Icon: Activity },
  { name: "libraries", to: "/libraries", label: "Libraries", Icon: FolderOpen },
  { name: "settings", to: "/settings", label: "Settings", Icon: Settings },
];

export default function Sidebar() {
  const route = useRouter((s) => s.route);
  const counts = useStatus((s) => s.status?.counts);
  const jobs = useStatus((s) => s.status?.jobs);
  const resolved = useThemeStore((s) => s.resolved);
  const profile = useProfile((s) => s.current);
  const admin = useIsAdmin();

  // Detail pages highlight the section they belong to.
  const liveTv = useStatus((s) => s.status?.livetv);
  const current: NavName =
    route.name === "item" || route.name === "play" || route.name === "person"
      ? "home"
      : route.name === "watch" || route.name === "recording"
        ? "livetv"
        : route.name === "manage"
          ? "libraries"
          : route.name;

  const badge = (name: NavName) => {
    if (name === "movies" && counts?.movies) return <Count n={counts.movies} />;
    if (name === "tv" && counts?.series) return <Count n={counts.series} />;
    if (name === "livetv" && liveTv?.recording)
      return (
        <span className="tint-critical inline-flex items-center gap-1 rounded px-1.5 text-[11px] font-semibold" title={`${liveTv.recording} recording now`}>
          <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-critical" />
          REC
        </span>
      );
    if (name === "activity" && jobs) {
      if (jobs.failed) return <Count n={jobs.failed} tint="tint-critical" title={`${jobs.failed} failed`} />;
      const active = jobs.queued + jobs.running;
      if (active) return <Count n={active} tint="tint-info" title={`${jobs.running} running, ${jobs.queued} queued`} />;
    }
    return null;
  };

  const item = ({ name, to, label, Icon }: (typeof ITEMS)[number]) => {
    const active = current === name;
    return (
      <Link
        key={name}
        to={to}
        aria-current={active ? "page" : undefined}
        className={`group flex w-full items-center gap-2.5 rounded-md px-2.5 py-1.5 text-sm transition-colors ${
          active ? "bg-muted font-medium text-content" : "text-content-secondary hover:bg-muted hover:text-content"
        }`}
      >
        <Icon size={16} className={active ? "text-accent" : "text-content-muted group-hover:text-content-secondary"} />
        <span className="flex-1 text-left">{label}</span>
        {badge(name)}
      </Link>
    );
  };

  return (
    <nav className="flex w-52 shrink-0 flex-col border-r border-border-light bg-surface px-2 py-3">
      <Link to="/" className="mb-5 flex items-center gap-2 px-2.5">
        <img src="/icons/logo-64.png" alt="" className="h-7 w-7" />
        <div className="leading-tight">
          <div className="text-sm font-semibold text-content">Couchside</div>
          <div className="text-[10px] font-medium uppercase tracking-widest brand-text">media</div>
        </div>
      </Link>
      <SearchBox />
      <div className="flex flex-col gap-0.5">
        {ITEMS.map(item)}
        {liveTv?.configured && item(LIVE)}
      </div>
      <div className="mb-1 mt-5 px-2.5 text-[10px] font-semibold uppercase tracking-widest text-content-muted">Manage</div>
      {/* Users only have their own settings to manage. */}
      <div className="flex flex-col gap-0.5">{MANAGE.filter((m) => admin || m.name === "settings").map(item)}</div>
      <div className="mt-auto flex flex-col gap-0.5 border-t border-border-light pt-2">
        {profile && (
          <Link
            to="/profiles"
            className="flex w-full items-center gap-2.5 rounded-md px-2.5 py-1.5 text-sm text-content-secondary hover:bg-muted hover:text-content"
            title="Switch profile"
          >
            <ProfileAvatar profile={profile} size={18} className="!rounded" />
            <span className="min-w-0 flex-1 truncate text-left">{profile.name}</span>
          </Link>
        )}
        <button
          onClick={() => setTheme(resolved === "dark" ? "light" : "dark")}
          className="flex w-full items-center gap-2.5 rounded-md px-2.5 py-1.5 text-sm text-content-secondary hover:bg-muted hover:text-content"
        >
          {resolved === "dark" ? <Sun size={16} className="text-content-muted" /> : <Moon size={16} className="text-content-muted" />}
          {resolved === "dark" ? "Light theme" : "Dark theme"}
        </button>
      </div>
    </nav>
  );
}

/**
 * Searches as you type: the first keystroke opens /search, later ones replace
 * its query so Back leaves the results instead of stepping through each letter.
 * "/" focuses it from anywhere.
 */
function SearchBox() {
  const route = useRouter((s) => s.route);
  const go = useRouter((s) => s.go);
  const routeQ = route.name === "search" ? route.q : "";
  const [q, setQ] = useState(routeQ);
  const ref = useRef<HTMLInputElement>(null);

  // Follow the URL (Back, leaving the page) unless someone is typing here.
  useEffect(() => {
    if (document.activeElement !== ref.current) setQ(routeQ);
  }, [routeQ]);

  const submit = (text: string) => {
    const onPage = useRouter.getState().route.name === "search";
    if (!text.trim() && !onPage) return;
    go(`/search?q=${encodeURIComponent(text)}`, { replace: onPage });
  };

  useEffect(() => {
    if (q === routeQ) return;
    const t = setTimeout(() => submit(q), 200);
    return () => clearTimeout(t);
    // Only typing triggers a search; routeQ changes come from it or from navigation.
  }, [q]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (e.key !== "/" || e.ctrlKey || e.metaKey || e.altKey || t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName)) return;
      e.preventDefault();
      ref.current?.focus();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <SearchInput
      ref={ref}
      type="search"
      value={q}
      onChange={setQ}
      placeholder="Search…"
      aria-label="Search"
      className="mb-4"
      onKeyDown={(e) => {
        if (e.key === "Enter") submit(q);
        if (e.key === "Escape") {
          setQ("");
          e.currentTarget.blur();
        }
      }}
    />
  );
}

function Count({ n, tint = "tint-muted", title }: { n: number; tint?: string; title?: string }) {
  return (
    <span className={`rounded px-1.5 text-[11px] font-semibold tabular-nums ${tint}`} title={title}>
      {n.toLocaleString()}
    </span>
  );
}
