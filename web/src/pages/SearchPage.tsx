import { useRef, useState } from "react";
import { Lock, Play, Search, SearchX } from "lucide-react";
import Link from "../components/Link";
import { ContinueCard, PosterRow, Row } from "../components/Rows";
import ProgramDialog from "../components/livetv/ProgramDialog";
import { RecDot } from "../components/livetv/ChannelBits";
import { EmptyState, ErrorNote, Loading, PageHeader, Section, Spinner } from "../components/ui";
import { SearchBox } from "../components/Sidebar";
import { useApi } from "../lib/api";
import { fmtSlot } from "../lib/format";
import type { Program, SearchResult, TvChannel } from "../lib/types";

/** The most each group shows (the server caps at 50); a full group means there may be more. */
const LIMIT = 50;

/** Results for the sidebar search box, grouped by kind. */
export default function SearchPage({ q }: { q: string }) {
  const term = q.trim();
  const { data, error, loading, reload } = useApi<SearchResult>(term ? `/api/search?q=${encodeURIComponent(term)}&limit=${LIMIT}` : null);
  // Keep the last results up while the next query loads, so typing doesn't flash.
  const last = useRef<SearchResult | undefined>(undefined);
  if (data) last.current = data;
  const res = term ? (data ?? last.current) : undefined;
  const [open, setOpen] = useState<{ p: Program; c: TvChannel | null } | null>(null);

  // Phones have no sidebar, so the box lives on the page there.
  const phoneBox = (
    <div className="gutter pt-3 md:hidden">
      <SearchBox hotkey={false} autoFocus={!term} className="" />
    </div>
  );

  if (!term)
    return (
      <>
        {phoneBox}
        <EmptyState icon={<Search size={36} strokeWidth={1.5} />} title="Search Couchside">
          Find movies, shows and episodes by title, and with live TV, channels and what's on the guide.
        </EmptyState>
      </>
    );

  const groups = res ? [res.movies, res.series, res.episodes, res.channels, res.programs] : [];
  const total = groups.reduce((n, g) => n + g.length, 0);
  const capped = groups.some((g) => g.length >= LIMIT);

  return (
    <div className="pb-8">
      {phoneBox}
      <PageHeader title={`Results for “${term}”`} subtitle={res && total ? `${total}${capped ? "+" : ""} found` : undefined}>
        {loading && <Spinner />}
      </PageHeader>
      {error && (
        <div className="gutter pt-4">
          <ErrorNote>{error}</ErrorNote>
        </div>
      )}
      {!res && loading && (
        <Loading />
      )}
      {res && total === 0 && !loading && <EmptyState icon={<SearchX size={36} strokeWidth={1.5} />} title={`Nothing matches “${term}”`}>Try fewer letters, or another spelling.</EmptyState>}
      {res && (
        <>
          <PosterRow title="Movies" items={res.movies} />
          <PosterRow title="TV shows" items={res.series} />
          {!!res.episodes.length && (
            <Row title="Episodes">
              {res.episodes.map((p) => (
                <ContinueCard key={p.fileId} p={p} />
              ))}
            </Row>
          )}
          {!!res.channels.length && (
            <Section title="Channels">
              <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
                {res.channels.map((c) => (
                  <ChannelHit key={c.number} c={c} />
                ))}
              </div>
            </Section>
          )}
          {!!res.programs.length && (
            <Section title="On the guide">
              <div className="card divide-y divide-border-light">
                {res.programs.map(({ program: p, channel: c }) => (
                  <button
                    key={p.id}
                    onClick={() => setOpen({ p, c })}
                    className="flex w-full items-center gap-3 px-3 py-2.5 text-left hover:bg-muted"
                  >
                    <span className="w-14 shrink-0 text-center text-xs font-semibold tabular-nums text-content-secondary">{p.channel}</span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium text-content">
                        <RecDot status={p.recordingStatus} className="mr-1.5" />
                        {p.title}
                        {p.episodeTitle && <span className="font-normal text-content-secondary"> · {p.episodeTitle}</span>}
                      </span>
                      <span className="block truncate text-xs text-content-muted">
                        {[fmtSlot(p.startAt, p.endAt), c?.name, p.isNew ? "New" : null].filter(Boolean).join(" · ")}
                      </span>
                    </span>
                  </button>
                ))}
              </div>
            </Section>
          )}
        </>
      )}
      {open && (
        <ProgramDialog
          // The fresh copy after a reload (Record, Don't record), not the one clicked.
          program={data?.programs.find((r) => r.program.id === open.p.id)?.program ?? open.p}
          channel={open.c ?? undefined}
          onClose={() => setOpen(null)}
          onChange={() => void reload()}
        />
      )}
    </div>
  );
}

function ChannelHit({ c }: { c: TvChannel }) {
  const body = (
    <>
      <div className="flex w-14 shrink-0 flex-col items-center gap-1">
        {c.logoUrl ? <img src={c.logoUrl} alt="" loading="lazy" className="channel-logo max-h-8 max-w-14" /> : <div className="h-8" />}
        <span className="text-xs font-semibold tabular-nums text-content-secondary">{c.number}</span>
      </div>
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium text-content">{c.name}</div>
        {c.affiliate && c.affiliate !== c.name && <div className="truncate text-xs text-content-muted">{c.affiliate}</div>}
      </div>
      {c.drm ? (
        <span className="badge tint-muted" title="This channel is encrypted (ATSC 3.0 DRM), so it can't be played">
          <Lock size={11} /> Copy-protected
        </span>
      ) : (
        <Play size={15} className="fill-current text-accent" />
      )}
    </>
  );
  if (c.drm)
    return (
      <div className="card flex items-center gap-3 p-3 opacity-70" tabIndex={0} aria-label={`${c.name}: copy-protected, can't be played`}>
        {body}
      </div>
    );
  return (
    <Link to={`/watch/${c.number}`} className="card flex items-center gap-3 p-3 hover:border-accent" aria-label={`Watch ${c.name}`}>
      {body}
    </Link>
  );
}
