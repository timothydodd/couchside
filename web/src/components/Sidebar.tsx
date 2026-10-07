import { ChevronDown, ChevronRight } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import Link from "./Link";
import ProfileAvatar from "./ProfileAvatar";
import { SECTIONS } from "./settings/sections";
import { SearchInput } from "./ui";
import { NAV_LIVE, NAV_MAIN, NAV_MANAGE, sectionOf, type NavItem, type NavName } from "./nav";
import { JobsBadge, RecBadge } from "./StatusBits";
import { useRouter } from "../stores/router";
import { useStatus } from "../stores/status";
import { useIsAdmin } from "../stores/auth";
import { useProfile } from "../stores/profile";

export default function Sidebar() {
  const route = useRouter((s) => s.route);
  const counts = useStatus((s) => s.status?.counts);
  const profile = useProfile((s) => s.current);
  const admin = useIsAdmin();

  const liveTv = useStatus((s) => s.status?.livetv);
  const current = sectionOf(route);

  const badge = (name: NavName) => {
    if (name === "movies" && counts?.movies) return <Count n={counts.movies} />;
    if (name === "tv" && counts?.series) return <Count n={counts.series} />;
    if (name === "livetv") return <RecBadge />;
    if (name === "activity") return <JobsBadge />;
    return null;
  };

  // Settings opens into its sections (admins only: users have just their own).
  const settingsOpen = admin && route.name === "settings";
  const section = route.name === "settings" ? route.section : null;

  const item = ({ name, to, label, Icon }: NavItem) => {
    // While Settings is open, the parent is "current" only on your own settings.
    const active = name === "settings" && settingsOpen ? section === "you" : current === name;
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
        {name === "settings" && admin && (settingsOpen ? <ChevronDown size={14} className="text-content-muted" /> : <ChevronRight size={14} className="text-content-muted" />)}
      </Link>
    );
  };

  return (
    <nav className="hidden w-52 shrink-0 flex-col md:flex border-r border-border-light bg-surface px-2 py-3">
      <Link to="/" className="mb-5 flex items-center gap-2 px-2.5">
        <img src="/icons/logo-64.png" alt="" className="h-7 w-7" />
        <div className="leading-tight">
          <div className="text-sm font-semibold text-content">Couchside</div>
          <div className="text-[10px] font-medium uppercase tracking-widest brand-text">media</div>
        </div>
      </Link>
      <SearchBox />
      <div className="flex flex-col gap-0.5">
        {NAV_MAIN.map(item)}
        {liveTv?.configured && item(NAV_LIVE)}
      </div>
      {/* Users only have their own settings, which isn't a group worth a heading. */}
      <div className={`mb-1 mt-5 px-2.5 text-[10px] font-semibold uppercase tracking-widest text-content-muted ${admin ? "" : "sr-only"}`}>{admin ? "Manage" : "Account"}</div>
      <div className="flex flex-col gap-0.5">
        {NAV_MANAGE.filter((m) => admin || !m.admin).map(item)}
        {settingsOpen && (
          <div className="subnav" aria-label="Settings sections">
            {SECTIONS.filter((s) => s.id !== "you").map(({ id, to, label, Icon }) => (
              <Link key={id} to={to} aria-current={section === id ? "page" : undefined} className="subnav-item">
                <Icon size={14} className={section === id ? "text-accent" : ""} />
                {label}
              </Link>
            ))}
          </div>
        )}
      </div>
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
      </div>
    </nav>
  );
}

/**
 * Searches as you type: the first keystroke opens /search, later ones replace
 * its query so Back leaves the results instead of stepping through each letter.
 * With hotkey, "/" focuses it from anywhere (the sidebar's; phones use the
 * Search page's own box).
 */
export function SearchBox({ hotkey = true, autoFocus = false, className = "mb-4" }: { hotkey?: boolean; autoFocus?: boolean; className?: string }) {
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
    if (!hotkey) return;
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (e.key !== "/" || e.ctrlKey || e.metaKey || e.altKey || t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName)) return;
      e.preventDefault();
      ref.current?.focus();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [hotkey]);

  return (
    <SearchInput
      ref={ref}
      type="search"
      value={q}
      onChange={setQ}
      placeholder="Search…"
      aria-label="Search"
      autoFocus={autoFocus}
      className={className}
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

function Count({ n }: { n: number }) {
  return <span className="badge tint-muted tabular-nums">{n.toLocaleString()}</span>;
}
