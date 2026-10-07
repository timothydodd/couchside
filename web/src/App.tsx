import { Suspense, useEffect, useLayoutEffect, useRef } from "react";
import { MobileTabBar, MobileTopBar } from "./components/MobileNav";
import Sidebar from "./components/Sidebar";
import StatusBar from "./components/StatusBar";
import { EmptyState, Spinner } from "./components/ui";
import { lazyPage } from "./lib/lazyPage";
import HomePage from "./pages/HomePage";
import ItemPage from "./pages/ItemPage";
import LibraryPage from "./pages/LibraryPage";
import LinkPage from "./pages/LinkPage";
import PersonPage from "./pages/PersonPage";
import EpisodePage from "./pages/EpisodePage";
import SearchPage from "./pages/SearchPage";
import ChangePasswordPage from "./pages/ChangePasswordPage";
import SetupPage from "./pages/SetupPage";
import SignInPage from "./pages/SignInPage";
import { useAuth, useIsAdmin } from "./stores/auth";
import { useProfile } from "./stores/profile";
import { startStatus } from "./stores/status";
import { useRouter } from "./stores/router";

// Loaded on first use: the players, Live TV and the admin pages are most of
// the code, and plenty of visits never open them.
const ActivityPage = lazyPage(() => import("./pages/ActivityPage"));
const LibrariesPage = lazyPage(() => import("./pages/LibrariesPage"));
const LibraryManagePage = lazyPage(() => import("./pages/LibraryManagePage"));
const PlayerPage = lazyPage(() => import("./pages/PlayerPage"));
const LiveTvPage = lazyPage(() => import("./pages/LiveTvPage"));
const LivePlayerPage = lazyPage(() => import("./pages/LivePlayerPage"));
const RecordingPlayerPage = lazyPage(() => import("./pages/RecordingPlayerPage"));
const SettingsPage = lazyPage(() => import("./pages/SettingsPage"));

export default function App() {
  const auth = useAuth();
  const route = useRouter((s) => s.route);

  useEffect(() => {
    void useAuth.getState().load();
  }, []);

  if (!auth.loaded) return null;
  if (auth.error) return <EmptyState title="Can't reach Couchside">{auth.error}</EmptyState>;
  if (auth.setupRequired) return <SetupPage />;
  if (!auth.user) return <SignInPage />;
  if (auth.user.mustChangePassword) return <ChangePasswordPage />;
  if (route.name === "profiles") return <SignInPage switching />;
  // /admin is where hidden admin accounts sign in. An admin has nothing more
  // to do there; anyone else can switch to an admin account.
  if (route.name === "admin") return auth.user.role === "admin" ? <Redirect to="/" /> : <SignInPage switching />;
  return <Signed />;
}

function Redirect({ to }: { to: string }) {
  const go = useRouter((s) => s.go);
  useEffect(() => go(to, { replace: true }), [go, to]);
  return null;
}

/** The app proper, once there's someone to show it to. */
// Where each page was scrolled to, so Back lands where you were (a long list
// of episodes, say) instead of at the top. Grid pages keep their own.
const scrollMemory = new Map<string, number>();

function Signed() {
  const route = useRouter((s) => s.route);
  const path = useRouter((s) => s.path);
  const pop = useRouter((s) => s.pop);
  const main = useRef<HTMLElement>(null);
  // Coming back: put the scroll position back once the page is tall enough
  // (its data may still be loading), giving up after a couple of seconds.
  useLayoutEffect(() => {
    const el = main.current;
    const want = pop ? scrollMemory.get(path) : undefined;
    if (!el || !want) return;
    const apply = () => {
      el.scrollTop = want;
      return Math.abs(el.scrollTop - want) < 2;
    };
    if (apply()) return;
    const ro = new ResizeObserver(() => apply() && ro.disconnect());
    for (const child of Array.from(el.children)) ro.observe(child);
    ro.observe(el);
    const stop = setTimeout(() => ro.disconnect(), 2000);
    return () => {
      ro.disconnect();
      clearTimeout(stop);
    };
  }, [path, pop]);
  const loaded = useProfile((s) => s.loaded);
  const admin = useIsAdmin();
  const adminOnly = route.name === "activity" || route.name === "libraries" || route.name === "manage";

  useEffect(() => {
    void useProfile.getState().load();
    startStatus();
  }, []);

  // Wait for the profile: everything shown (progress, favourites) belongs to it.
  if (!loaded) return null;

  // The player takes over the whole window.
  if (route.name === "play" || route.name === "watch" || route.name === "recording")
    return (
      <Suspense fallback={<div className="h-full bg-player-bg" />}>
        {route.name === "play" && <PlayerPage fileId={route.fileId} />}
        {route.name === "watch" && <LivePlayerPage channel={route.channel} />}
        {route.name === "recording" && <RecordingPlayerPage id={route.id} />}
      </Suspense>
    );

  return (
    <div className="flex h-full flex-col">
      <MobileTopBar />
      <div className="flex min-h-0 flex-1">
        <Sidebar />
        {/* Grid pages manage their own scroll (virtualised); the rest scroll here. */}
        <main
          ref={main}
          onScroll={(e) => scrollMemory.set(path, e.currentTarget.scrollTop)}
          key={route.name === "livetv" || route.name === "search" ? route.name : path}
          className={`min-w-0 flex-1 ${route.name === "movies" || route.name === "tv" || route.name === "livetv" ? "overflow-hidden" : "overflow-auto"}`}
        >
          <Suspense
            fallback={
              <div className="flex h-full items-center justify-center">
                <Spinner size={22} />
              </div>
            }
          >
          {route.name === "home" && <HomePage />}
          {route.name === "movies" && <LibraryPage kind="movie" />}
          {route.name === "tv" && <LibraryPage kind="series" />}
          {route.name === "item" && <ItemPage id={route.id} season={route.season} />}
          {route.name === "episode" && <EpisodePage id={route.id} />}
          {route.name === "livetv" && <LiveTvPage tab={route.tab} />}
          {adminOnly && !admin && <EmptyState title="Admins only">Ask an admin for access to this page.</EmptyState>}
          {route.name === "activity" && admin && <ActivityPage />}
          {route.name === "libraries" && admin && <LibrariesPage />}
          {route.name === "manage" && admin && <LibraryManagePage id={route.id} />}
          {route.name === "settings" && <SettingsPage />}
          {route.name === "search" && <SearchPage q={route.q} />}
          {route.name === "person" && <PersonPage id={route.id} />}
          {route.name === "link" && <LinkPage />}
          {route.name === "notfound" && <EmptyState title="Nothing here">That page doesn't exist.</EmptyState>}
          </Suspense>
        </main>
      </div>
      <StatusBar />
      <MobileTabBar />
    </div>
  );
}
