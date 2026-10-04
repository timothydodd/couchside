import { useEffect, useMemo, useRef, useState } from "react";
import { Clapperboard, FolderOpen, Search, Tv, X } from "lucide-react";
import FolderPicker from "../FolderPicker";
import { Choices } from "../FilterMenu";
import { ErrorNote, Segmented, Spinner } from "../ui";
import { api, useApi } from "../../lib/api";
import { fmtTime } from "../../lib/format";
import type { Library, Program, VirtualChannel, VirtualConfig, VirtualOptions } from "../../lib/types";
import { useDialog } from "../../lib/dialog";
import { errText } from "../../lib/errors";

const BLANK: VirtualConfig = {
  libraries: [],
  kinds: [],
  genres: [],
  excludeGenres: [],
  yearFrom: 0,
  yearTo: 0,
  minRating: 0,
  items: [],
  order: "shuffle",
  filler: { folder: "", align: 0, breakEvery: 0, breakLength: 120 },
};

/** Starting points for a new channel; everything stays editable. */
const PRESETS: { id: string; label: string; name: string; detail: string; config: Partial<VirtualConfig> }[] = [
  { id: "movies", label: "Movie night", name: "Movie Night", detail: "Movies, shuffled", config: { kinds: ["movie"] } },
  { id: "sitcoms", label: "Sitcom marathon", name: "Sitcoms", detail: "Comedies, episodes in order", config: { kinds: ["series"], genres: ["Comedy"], order: "sequential" } },
  { id: "decade", label: "A decade", name: "The 90s", detail: "Movies and shows from one decade", config: { yearFrom: 1990, yearTo: 1999 } },
  { id: "show", label: "One show", name: "", detail: "Every episode of a show, in order", config: { kinds: ["series"], order: "sequential" } },
  {
    id: "classic",
    label: "Classic TV",
    name: "Classic TV",
    detail: "Older shows with commercial breaks",
    config: { kinds: ["series"], yearTo: 1979, order: "sequential", filler: { folder: "", align: 30, breakEvery: 8, breakLength: 120 } },
  },
];

const KINDS: { id: "" | "movie" | "series"; label: string }[] = [
  { id: "", label: "Both" },
  { id: "movie", label: "Movies" },
  { id: "series", label: "Shows" },
];

/**
 * Create or edit one of Couchside's own channels: what it plays (filters or
 * picked titles), in what order, and optional commercials, with a preview of
 * the next few hours. Shown in the side drawer from Settings.
 */
export default function ChannelEditor({ channel, onClose, onSaved }: { channel?: VirtualChannel; onClose: () => void; onSaved: () => void }) {
  // Fresh: the next free number changes every time a channel is made.
  const { data: opts, error: optsError } = useApi<VirtualOptions>("/api/livetv/virtual/options", { fresh: true });
  const { data: libraries } = useApi<Library[]>("/api/libraries");
  const [number, setNumber] = useState(channel?.number ?? "");
  const [name, setName] = useState(channel?.name ?? "");
  const [cfg, setCfg] = useState<VirtualConfig>(channel ? { ...BLANK, ...channel.config, filler: { ...BLANK.filler, ...channel.config.filler } } : BLANK);
  const [preset, setPreset] = useState<string | null>(null);
  const [ads, setAds] = useState(!!channel?.config.filler?.folder);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  // Suggest a number once. After that the field is the user's: emptying it
  // to type another mustn't fill it in again.
  const suggested = useRef(false);
  useEffect(() => {
    if (channel || !opts || suggested.current) return;
    suggested.current = true;
    setNumber((n) => n || opts.nextNumber);
  }, [opts, channel]);
  const dialog = useRef<HTMLElement>(null);
  useDialog(dialog, onClose);

  const set = (patch: Partial<VirtualConfig>) => setCfg((c) => ({ ...c, ...patch }));
  const setFiller = (patch: Partial<VirtualConfig["filler"]>) => setCfg((c) => ({ ...c, filler: { ...c.filler, ...patch } }));
  const applyPreset = (id: string) => {
    const p = PRESETS.find((x) => x.id === id)!;
    setPreset(id);
    setCfg({ ...BLANK, ...p.config, filler: { ...BLANK.filler, ...p.config.filler } });
    setAds(!!p.config.filler?.breakEvery);
    if (p.name) setName(p.name);
  };
  // Commercials off means no folder, whatever was typed.
  const sent: VirtualConfig = ads ? cfg : { ...cfg, filler: { ...cfg.filler, folder: "" } };

  const save = async () => {
    if (ads && !cfg.filler.folder.trim()) {
      setErr("Choose the folder your commercials are in, or turn commercials off.");
      return;
    }
    setBusy(true);
    setErr(null);
    try {
      const r = await api<{ id: number; error: string }>(channel ? `/api/livetv/virtual/${channel.id}` : "/api/livetv/virtual", {
        method: channel ? "PUT" : "POST",
        json: { number, name, config: sent },
      });
      if (r.error) setErr(r.error);
      else {
        onSaved();
        onClose();
      }
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <div className="fixed inset-0 z-30 bg-backdrop/30" onClick={onClose} />
      <aside className="side-panel max-w-xl" role="dialog" aria-modal="true" ref={dialog} aria-label={channel ? `Edit ${channel.name}` : "New channel"}>
        <div className="flex items-center gap-3 px-4 py-4">
          <div className="min-w-0 flex-1">
            <div className="text-base font-semibold text-content">{channel ? `Edit ${channel.name}` : "New channel"}</div>
            <div className="text-xs text-content-muted">Plays around the clock from your library, in Live TV and the guide.</div>
          </div>
          <button className="btn-quiet" onClick={onClose} aria-label="Close">
            <X size={16} />
          </button>
        </div>
        <div className="flex-1 overflow-y-auto">
          {!channel && (
            <section className="panel-section">
              <div className="card-title mb-2">Start from</div>
              <div className="grid gap-2 sm:grid-cols-2">
                {PRESETS.map((p) => (
                  <button
                    key={p.id}
                    type="button"
                    onClick={() => applyPreset(p.id)}
                    aria-pressed={preset === p.id}
                    className={`rounded-lg border px-3 py-2 text-left transition-colors ${preset === p.id ? "border-accent bg-accent/10" : "border-border-light hover:border-border"}`}
                  >
                    <div className="text-sm font-medium text-content">{p.label}</div>
                    <div className="text-xs text-content-muted">{p.detail}</div>
                  </button>
                ))}
              </div>
            </section>
          )}

          <section className="panel-section">
            <div className="grid grid-cols-[6rem_1fr] gap-3">
              <label>
                <span className="field-label">Number</span>
                <input className="field w-full" value={number} onChange={(e) => setNumber(e.target.value)} inputMode="decimal" />
              </label>
              <label>
                <span className="field-label">Name</span>
                <input className="field w-full" value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. Saturday Morning" maxLength={60} />
              </label>
            </div>
          </section>

          <section className="panel-section flex flex-col gap-4">
            <div className="card-title">What plays</div>
            <div>
              <div className="field-label">Movies or shows</div>
              <Segmented
                label="Movies or shows"
                value={cfg.kinds.length === 1 ? cfg.kinds[0] : ""}
                onChange={(k) => set({ kinds: k ? [k] : [] })}
                options={KINDS}
              />
            </div>
            {libraries && libraries.length > 1 && (
              <div>
                <div className="field-label">Libraries</div>
                <div className="flex flex-wrap gap-1.5">
                  {libraries.map((l) => {
                    const on = cfg.libraries.includes(l.id);
                    return (
                      <button key={l.id} type="button" aria-pressed={on} className={`choice ${on ? "choice-on" : ""}`} onClick={() => set({ libraries: toggle(cfg.libraries, l.id) })}>
                        {l.name}
                      </button>
                    );
                  })}
                </div>
                <p className="mt-1 text-xs text-content-muted">{cfg.libraries.length ? "Only these libraries." : "All libraries."}</p>
              </div>
            )}
            {optsError && !opts && <ErrorNote>Couldn't load your library's genres, years and titles: {optsError}</ErrorNote>}
            {opts && opts.genres.length > 0 && (
              <>
                <GenreChips label="Any of these genres" all={opts.genres} value={cfg.genres} onChange={(genres) => set({ genres })} />
                <GenreChips label="Leave out" all={opts.genres} value={cfg.excludeGenres} onChange={(excludeGenres) => set({ excludeGenres })} />
              </>
            )}
            {opts && opts.minYear > 0 && <Years opts={opts} cfg={cfg} set={set} />}
            <Choices<string>
              label="Rating"
              value={String(cfg.minRating)}
              onChange={(v) => set({ minRating: Number(v) })}
              options={[
                { id: "0", label: "Any" },
                { id: "6", label: "6+" },
                { id: "7", label: "7+" },
                { id: "8", label: "8+" },
              ]}
            />
            {opts && <Titles opts={opts} cfg={cfg} set={set} />}
          </section>

          <section className="panel-section">
            <div className="card-title mb-2">Order</div>
            <Segmented
              label="Order"
              value={cfg.order}
              onChange={(order) => set({ order })}
              options={[
                { id: "shuffle", label: "Shuffle" },
                { id: "sequential", label: "In order" },
              ]}
            />
            <p className="mt-1 text-xs text-content-muted">
              {cfg.order === "shuffle"
                ? "Everything plays once, in random order, before anything repeats."
                : "Shows take turns, each continuing from its last episode; movies go oldest first."}
            </p>
          </section>

          <section className="panel-section">
            <label className="flex items-center gap-2 text-sm text-content">
              <input type="checkbox" className="accent-brand" checked={ads} onChange={(e) => setAds(e.target.checked)} />
              Add commercials
            </label>
            <p className="mt-1 text-xs text-content-muted">Clips from a folder (old ads, trailers, bumpers) between and inside programs. Optional.</p>
            {ads && <Commercials cfg={cfg} setFiller={setFiller} mediaRoot={opts?.mediaRoot ?? ""} />}
          </section>

          <Preview config={sent} />
        </div>
        <div className="flex items-center gap-2 border-t border-border-light px-4 py-3">
          <button className="btn-primary" disabled={busy || !name.trim() || !number.trim()} onClick={() => void save()}>
            {channel ? "Save" : "Create channel"}
          </button>
          <button className="btn-quiet" onClick={onClose}>
            Cancel
          </button>
          {err && <div className="min-w-0 flex-1 text-xs text-critical">{err}</div>}
        </div>
      </aside>
    </>
  );
}

function toggle<T>(list: T[], v: T): T[] {
  return list.includes(v) ? list.filter((x) => x !== v) : [...list, v];
}

function GenreChips({ label, all, value, onChange }: { label: string; all: string[]; value: string[]; onChange: (v: string[]) => void }) {
  return (
    <div>
      <div className="field-label">{label}</div>
      <div className="flex flex-wrap gap-1.5">
        {all.map((g) => {
          const on = value.includes(g);
          return (
            <button key={g} type="button" aria-pressed={on} className={`choice ${on ? "choice-on" : ""}`} onClick={() => onChange(toggle(value, g))}>
              {g}
            </button>
          );
        })}
      </div>
    </div>
  );
}

/** A year range, with a chip per decade in the library. */
function Years({ opts, cfg, set }: { opts: VirtualOptions; cfg: VirtualConfig; set: (p: Partial<VirtualConfig>) => void }) {
  const decades: number[] = [];
  for (let d = Math.floor(opts.minYear / 10) * 10; d <= opts.maxYear; d += 10) decades.push(d);
  const num = (v: string) => (v === "" ? 0 : Math.max(0, Math.min(9999, Number(v) || 0)));
  return (
    <div>
      <div className="field-label">Years</div>
      <div className="flex flex-wrap items-center gap-1.5">
        {decades.map((d) => {
          const on = cfg.yearFrom === d && cfg.yearTo === d + 9;
          return (
            <button key={d} type="button" aria-pressed={on} className={`choice ${on ? "choice-on" : ""}`} onClick={() => set(on ? { yearFrom: 0, yearTo: 0 } : { yearFrom: d, yearTo: d + 9 })}>
              {String(d).slice(-2)}s
            </button>
          );
        })}
      </div>
      <div className="mt-2 flex items-center gap-2 text-sm text-content-muted">
        <input className="field w-24" inputMode="numeric" placeholder="From" aria-label="From year" value={cfg.yearFrom || ""} onChange={(e) => set({ yearFrom: num(e.target.value) })} />
        to
        <input className="field w-24" inputMode="numeric" placeholder="To" aria-label="To year" value={cfg.yearTo || ""} onChange={(e) => set({ yearTo: num(e.target.value) })} />
      </div>
    </div>
  );
}

/** Pick specific movies and shows; when any are picked, only they play. */
function Titles({ opts, cfg, set }: { opts: VirtualOptions; cfg: VirtualConfig; set: (p: Partial<VirtualConfig>) => void }) {
  const [q, setQ] = useState("");
  const picked = opts.titles.filter((t) => cfg.items.includes(t.id));
  const found = useMemo(() => {
    const needle = q.trim().toLowerCase();
    if (!needle) return [];
    return opts.titles
      .filter((t) => !cfg.items.includes(t.id) && (cfg.kinds.length === 0 || cfg.kinds.includes(t.kind)) && t.title.toLowerCase().includes(needle))
      .slice(0, 8);
  }, [q, opts.titles, cfg.items, cfg.kinds]);
  return (
    <div>
      <div className="field-label">Only these titles</div>
      {picked.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-1.5">
          {picked.map((t) => (
            <button key={t.id} type="button" className="choice choice-on inline-flex items-center gap-1" aria-label={`Remove ${t.title}`} onClick={() => set({ items: toggle(cfg.items, t.id) })}>
              {t.title}
              <X size={12} />
            </button>
          ))}
        </div>
      )}
      <div className="relative">
        <Search size={14} className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-content-muted" />
        <input className="field w-full pl-8" placeholder="Find a movie or show to add" value={q} onChange={(e) => setQ(e.target.value)} aria-label="Find a title" />
      </div>
      {found.length > 0 && (
        <div className="mt-1 flex flex-col rounded-md border border-border-light">
          {found.map((t) => (
            <button
              key={t.id}
              type="button"
              className="flex items-center gap-2 px-3 py-2 text-left text-sm text-content hover:bg-muted"
              onClick={() => {
                set({ items: [...cfg.items, t.id] });
                setQ("");
              }}
            >
              {t.kind === "movie" ? <Clapperboard size={14} className="text-content-muted" /> : <Tv size={14} className="text-content-muted" />}
              <span className="flex-1 truncate">{t.title}</span>
              {t.year > 0 && <span className="text-xs text-content-muted">{t.year}</span>}
            </button>
          ))}
        </div>
      )}
      <p className="mt-1 text-xs text-content-muted">{picked.length ? "Only these play (the filters above still apply)." : "Leave empty to use the filters."}</p>
    </div>
  );
}

function Commercials({ cfg, setFiller, mediaRoot }: { cfg: VirtualConfig; setFiller: (p: Partial<VirtualConfig["filler"]>) => void; mediaRoot: string }) {
  const [browsing, setBrowsing] = useState(false);
  const f = cfg.filler;
  return (
    <div className="mt-3 flex flex-col gap-4">
      <div>
        <span className="field-label">Folder of clips</span>
        <div className="flex gap-2">
          <input className="field min-w-0 flex-1" value={f.folder} onChange={(e) => setFiller({ folder: e.target.value })} placeholder={mediaRoot ? `${mediaRoot}/Commercials` : "/media/Commercials"} />
          <button type="button" className="btn-ghost" onClick={() => setBrowsing((b) => !b)} aria-expanded={browsing}>
            <FolderOpen size={14} /> Browse
          </button>
        </div>
        {browsing && <FolderPicker path={f.folder || mediaRoot} onPick={(folder) => setFiller({ folder })} />}
        <p className="mt-1 text-xs text-content-muted">Every video in it (and its subfolders) can play. It doesn't need to be a library.</p>
      </div>
      <Choices<string>
        label="Start programs"
        value={String(f.align)}
        onChange={(v) => setFiller({ align: Number(v) })}
        options={[
          { id: "0", label: "Back to back" },
          { id: "15", label: "Every 15 min" },
          { id: "30", label: "On the half hour" },
          { id: "60", label: "On the hour" },
        ]}
      />
      <Choices<string>
        label="Breaks inside programs"
        value={String(f.breakEvery)}
        onChange={(v) => setFiller({ breakEvery: Number(v) })}
        options={[
          { id: "0", label: "None" },
          { id: "8", label: "Every 8 min" },
          { id: "12", label: "Every 12 min" },
          { id: "20", label: "Every 20 min" },
        ]}
      />
      {f.breakEvery > 0 && (
        <Choices<string>
          label="Each break lasts about"
          value={String(f.breakLength)}
          onChange={(v) => setFiller({ breakLength: Number(v) })}
          options={[
            { id: "60", label: "1 min" },
            { id: "120", label: "2 min" },
            { id: "180", label: "3 min" },
          ]}
        />
      )}
    </div>
  );
}

/** What the channel would play over the next few hours, refreshed as the settings change. */
function Preview({ config }: { config: VirtualConfig }) {
  const [res, setRes] = useState<{ programs: Program[]; matches: number } | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const key = JSON.stringify(config);
  useEffect(() => {
    let stale = false;
    const t = setTimeout(async () => {
      setLoading(true);
      try {
        const r = await api<{ programs: Program[]; matches: number }>("/api/livetv/virtual/preview?hours=6", { method: "POST", json: { config: JSON.parse(key) } });
        if (!stale) {
          setRes(r);
          setErr(null);
        }
      } catch (e) {
        if (!stale) setErr(errText(e));
      } finally {
        if (!stale) setLoading(false);
      }
    }, 400);
    return () => {
      stale = true;
      clearTimeout(t);
    };
  }, [key]);

  return (
    <section className="panel-section">
      <div className="mb-2 flex items-center gap-2">
        <div className="card-title flex-1">Preview</div>
        {loading && <Spinner />}
        {res && (
          <span className="text-xs text-content-muted">
            {res.matches} {res.matches === 1 ? "title" : "titles"} match
          </span>
        )}
      </div>
      {err && <ErrorNote>{err}</ErrorNote>}
      {res && res.matches === 0 && <p className="text-xs text-warning">Nothing in your library matches. Loosen the filters.</p>}
      {res && res.programs.length > 0 && (
        <ol className="flex flex-col divide-y divide-border-light rounded-md border border-border-light text-sm">
          {res.programs.slice(0, 10).map((p) => (
            <li key={p.startAt} className="flex items-baseline gap-3 px-3 py-1.5">
              <span className="w-16 shrink-0 text-xs tabular-nums text-content-muted">{fmtTime(p.startAt)}</span>
              <span className="min-w-0 flex-1 truncate text-content">
                {p.title}
                {p.episodeNum && <span className="text-content-muted"> · {p.episodeNum}</span>}
                {p.episodeTitle && <span className="text-content-muted"> · {p.episodeTitle}</span>}
              </span>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
