import { useEffect } from "react";
import Sidebar from "./components/Sidebar";
import StatusBar from "./components/StatusBar";
import { EmptyState } from "./components/ui";
import ActivityPage from "./pages/ActivityPage";
import HomePage from "./pages/HomePage";
import ItemPage from "./pages/ItemPage";
import LibrariesPage from "./pages/LibrariesPage";
import LibraryPage from "./pages/LibraryPage";
import LibraryManagePage from "./pages/LibraryManagePage";
import PlayerPage from "./pages/PlayerPage";
import LiveTvPage from "./pages/LiveTvPage";
import LivePlayerPage from "./pages/LivePlayerPage";
import RecordingPlayerPage from "./pages/RecordingPlayerPage";
import PersonPage from "./pages/PersonPage";
import SearchPage from "./pages/SearchPage";
import SettingsPage from "./pages/SettingsPage";
import ChangePasswordPage from "./pages/ChangePasswordPage";
import SetupPage from "./pages/SetupPage";
import SignInPage from "./pages/SignInPage";
import { useAuth, useIsAdmin } from "./stores/auth";
import { useProfile } from "./stores/profile";
import { startStatus } from "./stores/status";
import { useRouter } from "./stores/router";

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
  return <Signed />;
}

/** The app proper, once there's someone to show it to. */
function Signed() {
  const route = useRouter((s) => s.route);
  const path = useRouter((s) => s.path);
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
  if (route.name === "play") return <PlayerPage fileId={route.fileId} />;
  if (route.name === "watch") return <LivePlayerPage channel={route.channel} />;
  if (route.name === "recording") return <RecordingPlayerPage id={route.id} />;

  return (
    <div className="flex h-full flex-col">
      <div className="flex min-h-0 flex-1">
        <Sidebar />
        {/* Grid pages manage their own scroll (virtualised); the rest scroll here. */}
        <main
          key={route.name === "livetv" || route.name === "search" ? route.name : path}
          className={`min-w-0 flex-1 ${route.name === "movies" || route.name === "tv" || route.name === "livetv" ? "overflow-hidden" : "overflow-auto"}`}
        >
          {route.name === "home" && <HomePage />}
          {route.name === "movies" && <LibraryPage kind="movie" />}
          {route.name === "tv" && <LibraryPage kind="series" />}
          {route.name === "item" && <ItemPage id={route.id} />}
          {route.name === "livetv" && <LiveTvPage tab={route.tab} />}
          {adminOnly && !admin && <EmptyState title="Admins only">Ask an admin for access to this page.</EmptyState>}
          {route.name === "activity" && admin && <ActivityPage />}
          {route.name === "libraries" && admin && <LibrariesPage />}
          {route.name === "manage" && admin && <LibraryManagePage id={route.id} />}
          {route.name === "settings" && <SettingsPage />}
          {route.name === "search" && <SearchPage q={route.q} />}
          {route.name === "person" && <PersonPage id={route.id} />}
          {route.name === "notfound" && <EmptyState title="Nothing here">That page doesn't exist.</EmptyState>}
        </main>
      </div>
      <StatusBar />
    </div>
  );
}
