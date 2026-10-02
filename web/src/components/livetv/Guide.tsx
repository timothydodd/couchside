import { useEffect, useMemo, useRef, useState } from "react";
import { ChevronLeft, ChevronRight, Lock, Repeat } from "lucide-react";
import { PinButton, SignalBars } from "./ChannelBits";
import FilterBar from "./FilterBar";
import { channelNameMatches, channelPasses, genresOf, programFilterActive, programMatches, type TvFilters } from "./filters";
import ProgramDialog from "./ProgramDialog";
import { usePhone } from "../../lib/media";
import { useRouter } from "../../stores/router";
import { EmptyState, ErrorNote, Spinner } from "../ui";
import { useApi } from "../../lib/api";
import { fmtDay, fmtTime } from "../../lib/format";
import type { GuideResponse, Program, TvChannel } from "../../lib/types";

const PX_PER_MIN = 5; // 30 minutes = 150px
const HOURS = 4;
const ROW_H = 60;
const HALF_HOUR = 1800;

const floorHalfHour = (t: number) => Math.floor(t / HALF_HOUR) * HALF_HOUR;

/** Channels × time grid. Scrolls both ways; the channel column and time header stay pinned. */
export default function Guide({ filters, setFilters }: { filters: TvFilters; setFilters: (f: TvFilters) => void }) {
  const [start, setStart] = useState(() => floorHalfHour(Date.now() / 1000));
  const { data, error, loading, reload } = useApi<GuideResponse>(`/api/livetv/guide?start=${start}&hours=${HOURS}`, { pollMs: 60000 });
  const [open, setOpen] = useState<{ p: Program; c: TvChannel } | null>(null);
  const [now, setNow] = useState(() => Date.now() / 1000);
  const scroller = useRef<HTMLDivElement>(null);
  // Phones get a narrow channel column: number and name, no logo.
  const phone = usePhone();
  const CHANNEL_COL = phone ? 104 : 196;
  const go = useRouter((s) => s.go);

  useEffect(() => {
    const t = setInterval(() => setNow(Date.now() / 1000), 30000);
    return () => clearInterval(t);
  }, []);

  const end = start + HOURS * 3600;
  const width = HOURS * 60 * PX_PER_MIN;
  const ticks = useMemo(() => Array.from({ length: HOURS * 2 }, (_, i) => start + i * HALF_HOUR), [start]);
  const nowX = (now - start) / 60 * PX_PER_MIN;
  const atNow = start === floorHalfHour(now);

  const shift = (hours: number) => {
    setStart((s) => s + hours * 3600);
    scroller.current?.scrollTo({ left: 0 });
  };

  if (loading && !data)
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner size={22} />
      </div>
    );

  const empty = data && data.channels.every((c) => c.programs.length === 0);
  const genres = genresOf(data?.channels.flatMap((c) => c.programs) ?? []);
  const byProgram = programFilterActive(filters);
  // A channel shows when it passes the channel filters and, if a program filter
  // is on, its name matches the search or it has a matching program in view.
  const rows = (data?.channels ?? []).filter(
    (c) => channelPasses(c, filters) && (!byProgram || channelNameMatches(c, filters.q) || c.programs.some((p) => programMatches(p, filters))),
  );
  const lastPinned = rows.reduce((i, c, idx) => (c.pinned ? idx : i), -1);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex flex-wrap items-center gap-2 gutter py-3">
        <button className="btn-ghost" onClick={() => shift(-3)} aria-label="Earlier">
          <ChevronLeft size={15} />
        </button>
        <button className="btn-ghost" onClick={() => setStart(floorHalfHour(Date.now() / 1000))} disabled={atNow}>
          Now
        </button>
        <button className="btn-ghost" onClick={() => shift(3)} aria-label="Later" disabled={!!data && start + 3 * 3600 >= data.through}>
          <ChevronRight size={15} />
        </button>
        <span className="ml-1 text-sm font-medium text-content">
          {fmtDay(start)} · {fmtTime(start)} – {fmtTime(end)}
        </span>
        {data?.through ? <span className="ml-auto text-xs text-content-muted">Guide through {fmtDay(data.through)} {fmtTime(data.through)}</span> : null}
      </div>
      <div className="-mt-3">
        <FilterBar f={filters} set={setFilters} genres={genres} shown={rows.length} total={data?.channels.length ?? 0} />
      </div>
      {error && (
        <div className="gutter pb-3">
          <ErrorNote>{error}</ErrorNote>
        </div>
      )}
      {empty ? (
        <EmptyState title="No guide data for this time">The guide covers about a day ahead and refreshes every few hours.</EmptyState>
      ) : rows.length === 0 ? (
        <EmptyState title="No channels match">Try another search or genre, or look at a different time.</EmptyState>
      ) : (
        <div ref={scroller} className="min-h-0 flex-1 overflow-auto border-t border-border-light">
          <div className="relative" style={{ width: CHANNEL_COL + width }}>
            {/* time header */}
            <div className="sticky top-0 z-20 flex h-9 border-b border-border-light bg-surface">
              <div className="sticky left-0 z-30 shrink-0 border-r border-border-light bg-surface" style={{ width: CHANNEL_COL }} />
              <div className="relative" style={{ width }}>
                {ticks.map((t) => (
                  <div
                    key={t}
                    className="absolute top-0 flex h-full items-center border-l border-border-light pl-2 text-xs tabular-nums text-content-muted"
                    style={{ left: ((t - start) / 60) * PX_PER_MIN }}
                  >
                    {fmtTime(t)}
                  </div>
                ))}
              </div>
            </div>

            {rows.map((c, idx) => (
              <div key={c.number} className={`flex ${idx === lastPinned ? "border-b-2 border-b-warning/40" : "border-b border-border-light"}`} style={{ height: ROW_H }}>
                <div className="group sticky left-0 z-10 flex shrink-0 items-center border-r border-border-light bg-surface pr-1 hover:bg-raised" style={{ width: CHANNEL_COL }}>
                  <button
                    className="flex min-w-0 flex-1 items-center gap-2 self-stretch pl-2 text-left disabled:cursor-default md:gap-2.5 md:pl-3"
                    onClick={() => !c.drm && go(`/watch/${c.number}`)}
                    disabled={c.drm}
                    title={c.drm ? "Copy-protected channel" : `Watch ${c.name}`}
                  >
                    <div className="hidden h-8 w-12 shrink-0 items-center justify-center sm:flex">
                      {c.logoUrl ? <img src={c.logoUrl} alt="" loading="lazy" className="channel-logo max-h-8 max-w-12" /> : null}
                    </div>
                    <div className="min-w-0 leading-tight">
                      <div className="flex items-center gap-1.5 text-sm font-semibold tabular-nums text-content">
                        {c.number}
                        <SignalBars c={c} />
                        {c.hd && <span className="rounded bg-muted px-1 text-[9px] font-bold text-content-muted">HD</span>}
                      </div>
                      <div className="flex items-center gap-1 truncate text-xs text-content-muted group-hover:text-content-secondary">
                        {c.drm && <Lock size={10} />}
                        {c.name}
                      </div>
                    </div>
                  </button>
                  <PinButton c={c} onChange={() => void reload()} />
                </div>
                <div className="relative" style={{ width }}>
                  {c.programs.map((p) => {
                    const s = Math.max(p.startAt, start);
                    const e = Math.min(p.endAt, end);
                    if (e <= s) return null;
                    const left = ((s - start) / 60) * PX_PER_MIN + 2;
                    const w = ((e - s) / 60) * PX_PER_MIN - 4;
                    const onNow = p.startAt <= now && p.endAt > now;
                    const past = p.endAt <= now;
                    const dim = byProgram && !channelNameMatches(c, filters.q) && !programMatches(p, filters);
                    return (
                      <button
                        key={p.id}
                        className={`guide-cell ${onNow ? "guide-cell-now" : ""} ${past ? "guide-cell-past" : ""} ${dim ? "guide-cell-dim" : ""}`}
                        style={{ left, width: Math.max(w, 6) }}
                        onClick={() => setOpen({ p, c })}
                        title={`${p.title}${p.episodeTitle ? ` · ${p.episodeTitle}` : ""}`}
                      >
                        <div className="flex min-w-0 items-center gap-1.5">
                          {(p.recordingStatus === "scheduled" || p.recordingStatus === "recording") && (
                            <span className={`rec-dot ${p.recordingStatus === "recording" ? "animate-pulse" : ""}`} />
                          )}
                          <span className="truncate text-sm font-medium text-content">{p.title}</span>
                          {p.ruleId && <Repeat size={11} className="shrink-0 text-accent" aria-label="Series recording" />}
                          {p.isNew && w > 140 && <span className="tint-good shrink-0 rounded px-1 text-[10px] font-bold">NEW</span>}
                        </div>
                        {w > 90 && (
                          <div className="truncate text-xs text-content-muted">
                            {p.episodeTitle || `${fmtTime(p.startAt)} – ${fmtTime(p.endAt)}`}
                          </div>
                        )}
                      </button>
                    );
                  })}
                </div>
              </div>
            ))}

            {/* now line */}
            {nowX > 0 && nowX < width && (
              <div className="pointer-events-none absolute bottom-0 top-0 z-[5] w-0.5 bg-brand-seafoam/80" style={{ left: CHANNEL_COL + nowX }}>
                <div className="sticky top-9 -ml-[3px] h-2 w-2 rounded-full bg-brand-seafoam" />
              </div>
            )}
          </div>
        </div>
      )}
      {open && (
        <ProgramDialog
          program={data?.channels.find((c) => c.number === open.c.number)?.programs.find((p) => p.id === open.p.id) ?? open.p}
          channel={open.c}
          onClose={() => setOpen(null)}
          onChange={() => void reload()}
        />
      )}
    </div>
  );
}
