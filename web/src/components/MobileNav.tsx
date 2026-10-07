import { useEffect, useRef, useState } from "react";
import { LogOut, Menu, Search, Users, X, type LucideIcon } from "lucide-react";
import Link from "./Link";
import ProfileAvatar from "./ProfileAvatar";
import { NAV_LIVE, NAV_MAIN, NAV_MANAGE, sectionOf } from "./nav";
import { JobsBadge, RecBadge, useJobsSummary } from "./StatusBits";
import { useAuth, useIsAdmin } from "../stores/auth";
import { useProfile } from "../stores/profile";
import { useRouter } from "../stores/router";
import { useStatus } from "../stores/status";
import { useDialog } from "../lib/dialog";
import { attempt } from "../lib/notices";
import { fmtVersion } from "../lib/format";

/*
 * Phone layout (below md): a top bar with the logo, search and the profile,
 * and a bottom tab bar for the main sections. Everything the desktop sidebar
 * and status bar hold that doesn't fit is in the "More" sheet.
 */

export function MobileTopBar() {
  const profile = useProfile((s) => s.current);
  return (
    <header className="mobile-bar flex shrink-0 items-center gap-2 border-b border-border-light bg-surface md:hidden">
      <Link to="/" className="flex items-center gap-2" aria-label="Couchside home">
        <img src="/icons/logo-64.png" alt="" className="h-7 w-7" />
        <span className="text-base font-semibold text-content">Couchside</span>
      </Link>
      <RecBadge to="/livetv/recordings" className="ml-1" />
      <div className="flex-1" />
      <Link to="/search" className="touch-target text-content-secondary hover:text-content" aria-label="Search">
        <Search size={20} />
      </Link>
      {profile && (
        <Link to="/profiles" className="touch-target" aria-label={`Switch profile (now ${profile.name})`}>
          <ProfileAvatar profile={profile} size={28} />
        </Link>
      )}
    </header>
  );
}

export function MobileTabBar() {
  const route = useRouter((s) => s.route);
  const liveTv = useStatus((s) => s.status?.livetv?.configured);
  const [more, setMore] = useState(false);
  const current = sectionOf(route);
  const tabs = liveTv ? [...NAV_MAIN, NAV_LIVE] : NAV_MAIN;
  const inMore = NAV_MANAGE.some((m) => m.name === current);

  // Any navigation closes the sheet.
  useEffect(() => setMore(false), [route]);

  return (
    <>
      <nav aria-label="Main" className="mobile-tabs flex shrink-0 border-t border-border-light bg-surface md:hidden">
        {tabs.map(({ name, to, label, short, Icon }) => {
          const active = current === name;
          return (
            <Link key={name} to={to} aria-current={active ? "page" : undefined} className={`mobile-tab ${active ? "text-accent" : "text-content-muted"}`}>
              <Icon size={22} />
              <span>{short ?? label}</span>
            </Link>
          );
        })}
        <button type="button" onClick={() => setMore(true)} aria-expanded={more} className={`mobile-tab ${inMore ? "text-accent" : "text-content-muted"}`}>
          <Menu size={22} />
          <span>More</span>
        </button>
      </nav>
      {more && <MoreSheet onClose={() => setMore(false)} />}
    </>
  );
}

/** The rest of the navigation, plus who's signed in and what the server is doing. */
function MoreSheet({ onClose }: { onClose: () => void }) {
  const admin = useIsAdmin();
  const logout = useAuth((s) => s.logout);
  const profile = useProfile((s) => s.current);
  const status = useStatus((s) => s.status);
  const { label } = useJobsSummary();

  const sheet = useRef<HTMLDivElement>(null);
  useDialog(sheet, onClose);

  const row = (to: string, label: string, Icon: LucideIcon, extra?: React.ReactNode) => (
    <Link to={to} className="sheet-row">
      <Icon size={20} className="text-content-muted" />
      <span className="flex-1">{label}</span>
      {extra}
    </Link>
  );

  return (
    <div className="fixed inset-0 z-50 flex flex-col justify-end bg-backdrop/55 md:hidden" onClick={onClose}>
      <div ref={sheet} role="dialog" aria-modal="true" aria-label="More" className="sheet" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center gap-3 px-4 pb-2 pt-4">
          {profile && <ProfileAvatar profile={profile} size={36} />}
          <div className="min-w-0 flex-1">
            <div className="truncate font-medium text-content">{profile?.name}</div>
            <div className="text-xs text-content-muted">{admin ? "Admin" : "Signed in"}</div>
          </div>
          <button type="button" onClick={onClose} className="touch-target text-content-muted" aria-label="Close">
            <X size={20} />
          </button>
        </div>
        <div className="flex flex-col py-1">
          {NAV_MANAGE.filter((m) => admin || !m.admin).map((m) => row(m.to, m.label, m.Icon, m.name === "activity" ? <JobsBadge /> : undefined))}
          {row("/profiles", "Switch profile", Users)}
          <button type="button" className="sheet-row" onClick={attempt("Couldn't sign out", () => logout())}>
            <LogOut size={20} className="text-content-muted" />
            <span className="flex-1 text-left">Sign out</span>
          </button>
        </div>
        <div className="border-t border-border-light px-4 py-3 text-xs text-content-muted">
          {status ? label : "Connecting…"}
          {status && <span className="float-right mono">{fmtVersion(status.version)}</span>}
        </div>
      </div>
    </div>
  );
}
