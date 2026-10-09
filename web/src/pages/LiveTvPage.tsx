import { RadioTower } from "lucide-react";
import Link from "../components/Link";
import Channels from "../components/livetv/Channels";
import Guide from "../components/livetv/Guide";
import Recordings from "../components/livetv/Recordings";
import { useTvFilters } from "../components/livetv/filters";
import { EmptyState, PageHeader, WarningNote } from "../components/ui";
import { useApi } from "../lib/api";
import { useIsAdmin } from "../stores/auth";
import type { LiveTvStatus } from "../lib/types";
import { useTitle } from "../lib/title";

const TABS = [
  { tab: "guide", to: "/livetv", label: "Guide" },
  { tab: "channels", to: "/livetv/channels", label: "Channels" },
  { tab: "recordings", to: "/livetv/recordings", label: "Recordings" },
] as const;

export default function LiveTvPage({ tab }: { tab: "guide" | "channels" | "recordings" }) {
  const { data: st } = useApi<LiveTvStatus>("/api/livetv/status", { pollMs: 15000 });
  useTitle(tab === "guide" ? "Live TV" : tab === "channels" ? "Channels" : "Recordings");
  const [filters, setFilters] = useTvFilters();
  const admin = useIsAdmin();

  if (st && !st.configured)
    return (
      <EmptyState icon={<RadioTower size={36} strokeWidth={1.5} />} title="Live TV isn't set up">
        {admin ? (
          <>
            Make your own channels from your library in{" "}
            <Link to="/settings/livetv" className="text-accent hover:underline">
              Settings &rarr; Live TV
            </Link>
            , or set <span className="mono">COUCHSIDE_HDHOMERUN</span> to your HDHomeRun's IP address and restart Couchside for broadcast TV.
          </>
        ) : (
          "Ask an admin to add a tuner or make channels from the library."
        )}
      </EmptyState>
    );

  const tuners = st?.device?.TunerCount;
  const subtitle =
    st && !st.tuner
      ? "Your own channels, playing from the library"
      : st?.error
        ? `Can't reach the tuner: ${st.error}`
        : st?.device
          ? `${st.device.FriendlyName} · ${st.tunersInUse ?? 0} of ${tuners} tuners in use`
          : "Connecting to the tuner…";

  return (
    <div className="flex h-full flex-col">
      <PageHeader title="Live TV" subtitle={<span className={st?.error ? "text-critical" : undefined}>{subtitle}</span>} />
      <div className="gutter flex gap-1 overflow-x-auto border-b border-border-light">
        {TABS.map((t) => (
          <Link key={t.tab} to={t.to} aria-current={tab === t.tab ? "page" : undefined} className={`navtab shrink-0 !text-sm ${tab === t.tab ? "navtab-active" : ""}`}>
            {t.label}
            {t.tab === "recordings" && !!st?.recording && <span className="rec-dot ml-1.5 animate-pulse align-middle" />}
          </Link>
        ))}
      </div>
      {st?.guideError && (
        <div className="gutter pt-3">
          <WarningNote>Guide: {st.guideError}</WarningNote>
        </div>
      )}
      {tab === "guide" && <Guide filters={filters} setFilters={setFilters} />}
      {tab === "channels" && <Channels filters={filters} setFilters={setFilters} />}
      {tab === "recordings" && <Recordings />}
    </div>
  );
}
