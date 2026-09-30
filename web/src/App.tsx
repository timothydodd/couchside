import Sidebar from "./components/Sidebar";
import StatusBar from "./components/StatusBar";
import { EmptyState } from "./components/ui";
import ActivityPage from "./pages/ActivityPage";
import HomePage from "./pages/HomePage";
import ItemPage from "./pages/ItemPage";
import LibrariesPage from "./pages/LibrariesPage";
import LibraryPage from "./pages/LibraryPage";
import PlayerPage from "./pages/PlayerPage";
import LiveTvPage from "./pages/LiveTvPage";
import LivePlayerPage from "./pages/LivePlayerPage";
import RecordingPlayerPage from "./pages/RecordingPlayerPage";
import SettingsPage from "./pages/SettingsPage";
import { useRouter } from "./stores/router";

export default function App() {
  const route = useRouter((s) => s.route);
  const path = useRouter((s) => s.path);

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
          key={route.name === "livetv" ? "livetv" : path}
          className={`min-w-0 flex-1 ${route.name === "movies" || route.name === "tv" || route.name === "livetv" ? "overflow-hidden" : "overflow-auto"}`}
        >
          {route.name === "home" && <HomePage />}
          {route.name === "movies" && <LibraryPage kind="movie" />}
          {route.name === "tv" && <LibraryPage kind="series" />}
          {route.name === "item" && <ItemPage id={route.id} />}
          {route.name === "livetv" && <LiveTvPage tab={route.tab} />}
          {route.name === "activity" && <ActivityPage />}
          {route.name === "libraries" && <LibrariesPage />}
          {route.name === "settings" && <SettingsPage />}
          {route.name === "notfound" && <EmptyState title="Nothing here">That page doesn't exist.</EmptyState>}
        </main>
      </div>
      <StatusBar />
    </div>
  );
}
