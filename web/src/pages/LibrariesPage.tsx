import { useState } from "react";
import { Check, Clapperboard, Cpu, Folder, FolderPlus, Pencil, ScanSearch, Trash2, Tv } from "lucide-react";
import FolderPicker from "../components/FolderPicker";
import { EmptyState, ErrorNote, PageHeader, Spinner } from "../components/ui";
import { api, useApi } from "../lib/api";
import { fmtAgo } from "../lib/format";
import type { Library } from "../lib/types";
import { useStatus } from "../stores/status";

export default function LibrariesPage() {
  const { data, error, reload } = useApi<Library[]>("/api/libraries", { pollMs: 5000 });
  const [adding, setAdding] = useState(false);
  const [editing, setEditing] = useState<number | null>(null);

  const scan = async (id: number) => {
    await api(`/api/libraries/${id}/scan`, { method: "POST" });
    void useStatus.getState().refresh();
  };
  const optimize = async (l: Library) => {
    if (!confirm(`Encode browser-friendly copies of everything in "${l.name}" that can't play directly?\n\nThis runs in the background, one file at a time, and the copies take disk space in the cache volume (roughly 1-4 GB per movie at 1080p).`)) return;
    const r = await api<{ queued: number }>(`/api/libraries/${l.id}/optimize`, { method: "POST" });
    alert(r.queued ? `Queued ${r.queued} encodes. Follow them on the Activity page.` : "Nothing to do: everything already plays directly or has an optimized copy.");
    void useStatus.getState().refresh();
  };
  const remove = async (l: Library) => {
    if (!confirm(`Remove "${l.name}" from Couchside? Your files are not touched; only the library entry, watch history and artwork go.`)) return;
    await api(`/api/libraries/${l.id}`, { method: "DELETE" });
    await reload();
    void useStatus.getState().refresh();
  };

  return (
    <div>
      <PageHeader title="Libraries" subtitle="Folders Couchside scans for movies and TV shows">
        {!adding && (
          <button className="btn-primary" onClick={() => setAdding(true)}>
            <FolderPlus size={15} /> Add library
          </button>
        )}
      </PageHeader>
      <div className="flex flex-col gap-4 px-6 py-5">
        {error && <ErrorNote>{error}</ErrorNote>}
        {adding && (
          <LibraryForm
            onDone={() => {
              setAdding(false);
              void reload();
              void useStatus.getState().refresh();
            }}
            onCancel={() => setAdding(false)}
          />
        )}
        {data?.length === 0 && !adding && (
          <div className="card">
            <EmptyState icon={<Folder size={32} strokeWidth={1.5} />} title="No libraries yet">
              Add a folder of movies or TV shows to get started.
            </EmptyState>
          </div>
        )}
        {data?.map((l) =>
          editing === l.id ? (
            <LibraryForm
              key={l.id}
              library={l}
              onDone={() => {
                setEditing(null);
                void reload();
                void useStatus.getState().refresh();
              }}
              onCancel={() => setEditing(null)}
            />
          ) : (
            <div key={l.id} className="card flex flex-wrap items-center gap-4 px-4 py-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-md bg-muted text-accent">
                {l.kind === "movies" ? <Clapperboard size={18} /> : <Tv size={18} />}
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-semibold text-content">{l.name}</span>
                  <span className="tint-muted rounded px-1.5 py-0.5 text-[11px] font-semibold">{l.kind === "movies" ? "Movies" : "TV"}</span>
                </div>
                <div className="mono mt-0.5 truncate text-content-muted">{l.path}</div>
              </div>
              <div className="text-right text-xs text-content-secondary">
                <div className="tabular-nums">
                  {l.itemCount.toLocaleString()} {l.kind === "movies" ? "movies" : "shows"} · {l.fileCount.toLocaleString()} files
                </div>
                <div className="text-content-muted">Scanned {fmtAgo(l.lastScanAt)}</div>
              </div>
              <div className="flex gap-1.5">
                <button className="btn-ghost" onClick={() => void scan(l.id)}>
                  <ScanSearch size={15} /> Scan
                </button>
                <button className="btn-ghost" onClick={() => void optimize(l)} title="Encode browser-friendly copies of files that need transcoding">
                  <Cpu size={15} /> Optimize
                </button>
                <button className="btn-quiet" onClick={() => setEditing(l.id)} title="Rename or change folder">
                  <Pencil size={15} />
                </button>
                <button className="btn-quiet hover:!text-critical" onClick={() => void remove(l)} title="Remove library">
                  <Trash2 size={15} />
                </button>
              </div>
            </div>
          ),
        )}
      </div>
    </div>
  );
}

/** Add a library, or edit one's name and folder (its kind is fixed once added). */
function LibraryForm({ library, onDone, onCancel }: { library?: Library; onDone: () => void; onCancel: () => void }) {
  const mediaRoot = useStatus((s) => s.status?.mediaRoot);
  const [name, setName] = useState(library?.name ?? "");
  const [kind, setKind] = useState<"movies" | "tv">(library?.kind ?? "movies");
  const [path, setPath] = useState(library?.path ?? mediaRoot ?? "");
  const moving = !!library && path.trim().replace(/\/+$/, "") !== library.path;
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    setBusy(true);
    setErr(null);
    try {
      const n = name.trim() || (kind === "movies" ? "Movies" : "TV Shows");
      if (library) await api(`/api/libraries/${library.id}`, { method: "PUT", json: { name: n, path } });
      else await api("/api/libraries", { method: "POST", json: { name: n, path, kind } });
      onDone();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="card p-4">
      <div className="card-title mb-3">{library ? `Edit ${library.name}` : "New library"}</div>
      <div className="grid gap-3 md:grid-cols-[1fr_180px]">
        <div>
          <label className="field-label" htmlFor="lib-name">
            Name
          </label>
          <input id="lib-name" className="field w-full" placeholder={kind === "movies" ? "Movies" : "TV Shows"} value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div>
          <label className="field-label" htmlFor="lib-kind">
            Contains
          </label>
          <select
            id="lib-kind"
            className="field w-full"
            value={kind}
            disabled={!!library}
            title={library ? "Fixed once added: to switch, remove the library and add it again" : undefined}
            onChange={(e) => setKind(e.target.value as "movies" | "tv")}
          >
            <option value="movies">Movies</option>
            <option value="tv">TV shows</option>
          </select>
        </div>
      </div>
      <div className="mt-3">
        <label className="field-label" htmlFor="lib-path">
          Folder
        </label>
        <input id="lib-path" className="field mono w-full" placeholder="/media/movies" value={path} onChange={(e) => setPath(e.target.value)} />
        {mediaRoot && <FolderPicker path={path || mediaRoot} onPick={setPath} />}
        <p className="mt-1.5 text-xs text-content-muted">
          {moving
            ? "Files found at the same place under the new folder keep their watch history and artwork; anything missing is removed on the rescan."
            : kind === "movies"
            ? "One movie per file or folder, e.g. Inception (2010)/Inception (2010).mkv"
            : "One folder per show, e.g. Severance/Season 1/Severance S01E01.mkv"}
        </p>
      </div>
      {err && <div className="mt-3"><ErrorNote>{err}</ErrorNote></div>}
      <div className="mt-4 flex gap-2">
        <button className="btn-primary" disabled={busy || !path.trim()} onClick={() => void submit()}>
          {busy ? <Spinner size={14} /> : library ? <Check size={15} /> : <FolderPlus size={15} />}{" "}
          {library ? (moving ? "Save and rescan" : "Save") : "Add and scan"}
        </button>
        <button className="btn-quiet" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </div>
  );
}
