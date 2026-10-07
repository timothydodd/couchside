import { useState } from "react";
import { Check, FolderPlus } from "lucide-react";
import FolderPicker from "./FolderPicker";
import { ErrorNote, Spinner } from "./ui";
import { api } from "../lib/api";
import { errText } from "../lib/errors";
import type { Library } from "../lib/types";
import { useStatus } from "../stores/status";

/** Add a library, or edit one's name and folder (its kind is fixed once added). */
export default function LibraryForm({ library, onDone, onCancel }: { library?: Library; onDone: () => void; onCancel: () => void }) {
  const mediaRoot = useStatus((s) => s.status?.mediaRoot);
  const [name, setName] = useState(library?.name ?? "");
  const [kind, setKind] = useState<"movies" | "tv">(library?.kind ?? "movies");
  const [path, setPath] = useState(library?.path ?? mediaRoot ?? "");
  const [trickplay, setTrickplay] = useState(library?.trickplay ?? false);
  const [intros, setIntros] = useState(library?.intros ?? false);
  const moving = !!library && path.trim().replace(/\/+$/, "") !== library.path;
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    setBusy(true);
    setErr(null);
    try {
      const n = name.trim() || (kind === "movies" ? "Movies" : "TV Shows");
      if (library) await api(`/api/libraries/${library.id}`, { method: "PUT", json: { name: n, path, trickplay, intros } });
      else {
        const made = await api<Library>("/api/libraries", { method: "POST", json: { name: n, path, kind } });
        if (trickplay || intros) await api(`/api/libraries/${made.id}`, { method: "PUT", json: { name: n, path: made.path, trickplay, intros } });
      }
      onDone();
    } catch (e) {
      setErr(errText(e));
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
      <label className="mt-3 flex items-start gap-2 text-sm">
        <input type="checkbox" className="mt-1" checked={trickplay} onChange={(e) => setTrickplay(e.target.checked)} />
        <span>
          Preview thumbnails on the seek bar
          <span className="block text-xs text-content-muted">
            Each file is read from start to end once to make them (a few minutes for a large film on a network share). They run after other work, and show in
            Activity.
          </span>
        </span>
      </label>
      {kind === "tv" && (
        <label className="mt-3 flex items-start gap-2 text-sm">
          <input type="checkbox" className="mt-1" checked={intros} onChange={(e) => setIntros(e.target.checked)} />
          <span>
            Find intros, for a Skip intro button
            <span className="block text-xs text-content-muted">
              Listens to the first minutes of each season's episodes for the opening they share. Files with named chapters don't need it.
            </span>
          </span>
        </label>
      )}
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
