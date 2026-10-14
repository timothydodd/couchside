import { useRef } from "react";
import Link from "../components/Link";
import AccountManager from "../components/settings/AccountManager";
import AdvancedArea from "../components/settings/AdvancedArea";
import Diagnostics from "../components/settings/Diagnostics";
import SingleSignOn from "../components/settings/SingleSignOn";
import AccountSettings from "../components/settings/AccountSettings";
import Console from "../components/settings/Console";
import LiveTvSettings from "../components/settings/LiveTvSettings";
import MetadataSettings from "../components/settings/MetadataSettings";
import ProfileSettings from "../components/settings/ProfileSettings";
import { SECTIONS } from "../components/settings/sections";
import ServerInfo from "../components/settings/ServerInfo";
import ServerNow from "../components/settings/ServerNow";
import ServerSettings from "../components/settings/ServerSettings";
import SystemHistory from "../components/settings/SystemHistory";
import BackupSettings from "../components/settings/Backups";
import TimingSettings from "../components/settings/TimingSettings";
import VirtualChannels from "../components/settings/VirtualChannels";
import { PageHeader } from "../components/ui";
import { usePhone } from "../lib/media";
import { useScrollEdges } from "../lib/scroll";
import { useIsAdmin } from "../stores/auth";
import { useRouter } from "../stores/router";
import { useStatus } from "../stores/status";
import { fmtVersion } from "../lib/format";
import { useTitle } from "../lib/title";

/**
 * Settings. /settings is your own preferences and account, for every profile;
 * the server's settings are admin-only sections of their own, listed under
 * Settings in the sidebar (tabs on phones).
 */
export default function SettingsPage() {
  const route = useRouter((s) => s.route);
  const admin = useIsAdmin();
  const phone = usePhone();
  // Users only have their own settings; any other section shows those.
  const id = admin && route.name === "settings" ? route.section : "you";
  const section = SECTIONS.find((s) => s.id === id)!;
  useTitle(id === "you" ? "Settings" : `${section.label} · Settings`);
  const wide = id === "system" || id === "console";
  const tabs = useRef<HTMLElement>(null);
  useScrollEdges(tabs); // fades the end of the phone tab bar that has more

  return (
    <div>
      <PageHeader title={id === "you" ? "Settings" : section.label} subtitle={section.blurb} />
      {admin && phone && (
        <nav ref={tabs} className="row-scroll gutter flex gap-1 overflow-x-auto border-b border-border-light" aria-label="Settings sections">
          {SECTIONS.map((s) => (
            <Link key={s.id} to={s.to} aria-current={s.id === id ? "page" : undefined} className={`navtab shrink-0 whitespace-nowrap !text-sm ${s.id === id ? "navtab-active" : ""}`}>
              {s.id === "you" ? "You" : s.label}
            </Link>
          ))}
        </nav>
      )}
      <div className={`flex flex-col gap-4 gutter py-5 ${wide ? "max-w-5xl" : "max-w-3xl"}`}>
        {id === "you" && (
          <>
            <ProfileSettings />
            <AccountSettings />
          </>
        )}
        {id === "system" && (
          <>
            <ServerNow />
            <SystemHistory />
            <ServerInfo />
            <AdvancedArea what="Backups of the database, and diagnostics for a bug report">
              <BackupSettings />
              <Diagnostics />
            </AdvancedArea>
          </>
        )}
        {id === "server" && <ServerSettings />}
        {id === "console" && <Console />}
        {id === "accounts" && (
          <>
            <AccountManager />
            <AdvancedArea what="Single sign-on through an identity provider">
              <SingleSignOn />
            </AdvancedArea>
          </>
        )}
        {id === "metadata" && <MetadataSettings />}
        {id === "livetv" && (
          <>
            <LiveTvSettings />
            <VirtualChannels />
            <AdvancedArea what="Recording padding and commercial-skip timing">
              <TimingSettings />
            </AdvancedArea>
          </>
        )}
        {id === "you" && <About />}
      </div>
    </div>
  );
}

/** Version, license, credits and the third-party notices, at the bottom of Settings. */
function About() {
  const version = useStatus((s) => s.status?.version);
  const apiVersion = useStatus((s) => s.status?.apiVersion);
  const providers = useStatus((s) => s.status?.providers);
  const link = "hover:text-accent hover:underline";
  return (
    <div className="flex flex-col gap-2 px-1 text-xs text-content-muted">
      <p>
        Couchside {version && fmtVersion(version)}
        {apiVersion ? ` (API ${apiVersion})` : ""} &middot; free, open-source software under the{" "}
        <a href="https://github.com/timothydodd/couchside/blob/main/LICENSE" target="_blank" rel="noreferrer" className={link}>
          MIT License
        </a>
        , with no warranty &middot;{" "}
        <a href="/third-party-notices.txt" target="_blank" rel="noreferrer" className={link}>
          Open-source licenses
        </a>
      </p>
      <p>Couchside doesn't provide or find any content. Use it only with media you have the right to play.</p>
      {providers?.includes("tmdb") && (
        <p className="flex items-center gap-2">
          <a href="https://www.themoviedb.org" target="_blank" rel="noreferrer" className="shrink-0" aria-label="The Movie Database">
            <img src="/brand/tmdb.svg" alt="TMDB" className="h-3" />
          </a>
          <span>This product uses the TMDB API but is not endorsed or certified by TMDB.</span>
        </p>
      )}
      {providers?.includes("omdb") && (
        <p>
          Some details from{" "}
          <a href="https://www.omdbapi.com" target="_blank" rel="noreferrer" className={link}>
            OMDb
          </a>{" "}
          under{" "}
          <a href="https://creativecommons.org/licenses/by-nc/4.0/" target="_blank" rel="noreferrer" className={link}>
            CC BY-NC 4.0
          </a>
          .
        </p>
      )}
      <p>Product names such as Roku, HDHomeRun and Dolby Vision are trademarks of their owners, who don't endorse Couchside.</p>
    </div>
  );
}
