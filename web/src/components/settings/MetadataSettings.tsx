import { CheckCircle2, XCircle } from "lucide-react";
import { useStatus } from "../../stores/status";
import { WarningNote } from "../ui";

/** Where titles, plots and artwork come from: TMDB first, OMDb as a fallback. */
export default function MetadataSettings() {
  const status = useStatus((s) => s.status);
  const tmdb = status?.providers.includes("tmdb");
  const omdb = status?.providers.includes("omdb");
  return (
    <section className="card p-4">
      <div className="card-title mb-3">Metadata</div>
      {!!status?.metadataDegraded?.length && (
        <div className="mb-3">
          <WarningNote>
            {status.metadataDegraded.join(". ")}.{" "}
            {status.tmdbKey === "builtin"
              ? "Couchside's shared key is being refused, so new titles won't match. Set your own TMDB API key (free from themoviedb.org) in Settings → Server; titles match on the next scan."
              : "Check the TMDB API key in Settings → Server."}
          </WarningNote>
        </div>
      )}
      <div className="flex flex-col gap-4">
        <div className="flex items-start gap-3">
          {tmdb ? <CheckCircle2 size={18} className="mt-0.5 text-good" /> : <XCircle size={18} className="mt-0.5 text-content-muted" />}
          <div className="text-sm">
            <div className="font-medium text-content">The Movie Database (TMDB)</div>
            <p className="mt-0.5 text-xs text-content-muted">
              {!tmdb
                ? "Off. Set TMDB_API_KEY on the server (a free key from themoviedb.org), or remove TMDB_API_KEY=off."
                : status?.tmdbKey === "builtin"
                  ? "Connected with Couchside's shared key: titles, plots, posters, backdrops and episode names, cached for up to 30 days. Every Couchside without its own key uses it, so TMDB sees this server's address under it. Set your own TMDB API key in Settings → Server to stop relying on it."
                  : "Connected with your TMDB_API_KEY: titles, plots, posters, backdrops and episode names, cached for up to 30 days."}
            </p>
          </div>
        </div>
        <div className="flex items-start gap-3">
          {omdb ? <CheckCircle2 size={18} className="mt-0.5 text-good" /> : <XCircle size={18} className="mt-0.5 text-content-muted" />}
          <div className="text-sm">
            <div className="font-medium text-content">Open Movie Database (OMDb)</div>
            <p className="mt-0.5 text-xs text-content-muted">
              {omdb
                ? tmdb
                  ? "Connected as a fallback for titles TMDB can't place."
                  : "Connected. Titles, plots, IMDb ratings and posters (no backdrops); responses are cached for 30 days."
                : "Optional. Set OMDB_API_KEY (free at omdbapi.com) for a second source."}
            </p>
          </div>
        </div>
      </div>
      {tmdb && (
        <div className="mt-4 flex items-center gap-3 border-t border-border-light pt-3 text-xs text-content-muted">
          <a href="https://www.themoviedb.org" target="_blank" rel="noreferrer" className="shrink-0" aria-label="The Movie Database">
            <img src="/brand/tmdb.svg" alt="TMDB" className="h-7" />
          </a>
          <span>This product uses the TMDB API but is not endorsed or certified by TMDB.</span>
        </div>
      )}
    </section>
  );
}
