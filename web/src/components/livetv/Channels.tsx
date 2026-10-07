import { useState } from "react";
import { Lock, Play } from "lucide-react";
import Link from "../Link";
import { PinButton, SignalBars } from "./ChannelBits";
import FilterBar from "./FilterBar";
import { channelNameMatches, channelPasses, genresOf, programFilterActive, programMatches, type TvFilters } from "./filters";
import ProgramDialog from "./ProgramDialog";
import { EmptyState, ErrorNote, Meter, Loading } from "../ui";
import { useApi } from "../../lib/api";
import { fmtTime } from "../../lib/format";
import type { ChannelNow, Program, TvChannel } from "../../lib/types";

/** What's on now, one card per channel. */
export default function Channels({ filters, setFilters }: { filters: TvFilters; setFilters: (f: TvFilters) => void }) {
  const { data, error, loading, reload } = useApi<ChannelNow[]>("/api/livetv/channels", { pollMs: 60000 });
  const [open, setOpen] = useState<{ p: Program; c: TvChannel } | null>(null);
  const now = Date.now() / 1000;
  const byProgram = programFilterActive(filters);
  // Program filters look at what's on now and next.
  const list = (data ?? []).filter(
    (c) =>
      channelPasses(c, filters) &&
      (!byProgram || channelNameMatches(c, filters.q) || [c.now, c.next].some((p) => p && programMatches(p, filters))),
  );
  const genres = genresOf((data ?? []).flatMap((c) => [c.now, c.next].filter((p): p is Program => !!p)));

  if (loading && !data)
    return (
      <Loading fill />
    );

  return (
    <div className="min-h-0 flex-1 overflow-auto gutter pb-8">
      <div className="gutter-bleed">
        <FilterBar f={filters} set={setFilters} genres={genres} shown={list.length} total={data?.length ?? 0} />
      </div>
      {list.length === 0 && data && <EmptyState title="No channels match">Try another search or genre.</EmptyState>}
      {error && <ErrorNote>{error}</ErrorNote>}
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {list.map((c) => {
          const p = c.now;
          const pct = p ? ((now - p.startAt) / (p.endAt - p.startAt)) * 100 : 0;
          return (
            <div key={c.number} className={`card group flex gap-3 p-3 ${c.pinned ? "border-warning/40" : ""}`}>
              <div className="flex w-14 shrink-0 flex-col items-center gap-1 pt-0.5">
                {c.logoUrl ? <img src={c.logoUrl} alt="" loading="lazy" className="channel-logo max-h-8 max-w-14" /> : <div className="h-8" />}
                <span className="text-xs font-semibold tabular-nums text-content-secondary">{c.number}</span>
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-1.5 text-xs text-content-muted">
                  {c.drm && <Lock size={10} />}
                  {c.name}
                  {c.affiliate && c.affiliate !== c.name && <span>· {c.affiliate}</span>}
                  <SignalBars c={c} />
                  {c.hd && <span className="rounded bg-muted px-1 text-[9px] font-bold">HD</span>}
                  <PinButton c={c} onChange={() => void reload()} className="-my-1 ml-auto" />
                </div>
                {p ? (
                  <button className="mt-0.5 block w-full truncate text-left text-sm font-medium text-content hover:text-accent" onClick={() => setOpen({ p, c })}>
                    {(p.recordingStatus === "recording" || p.recordingStatus === "scheduled") && <span className="rec-dot mr-1.5" />}
                    {p.title}
                  </button>
                ) : (
                  <div className="mt-0.5 text-sm text-content-muted">No listing</div>
                )}
                {p && (
                  <div className="mt-1.5 flex items-center gap-2">
                    <Meter value={pct} className="flex-1" />
                    <span className="shrink-0 text-[11px] tabular-nums text-content-muted">{Math.max(0, Math.round((p.endAt - now) / 60))}m left</span>
                  </div>
                )}
                {c.next && (
                  <div className="mt-1 truncate text-xs text-content-muted">
                    Next {fmtTime(c.next.startAt)}: {c.next.title}
                  </div>
                )}
              </div>
              {!c.drm && (
                <Link to={`/watch/${c.number}`} className="btn-ghost self-center !px-2.5" aria-label={`Watch ${c.name}`}>
                  <Play size={15} className="fill-current" />
                </Link>
              )}
            </div>
          );
        })}
      </div>
      {open && (
        <ProgramDialog
          // The fresh copy after a reload (Record, Don't record), not the one clicked.
          program={[data?.find((c) => c.number === open.c.number)?.now, data?.find((c) => c.number === open.c.number)?.next].find((p) => p?.id === open.p.id) ?? open.p}
          channel={open.c}
          onClose={() => setOpen(null)}
          onChange={() => void reload()}
        />
      )}
    </div>
  );
}
