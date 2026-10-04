import { useEffect, useState } from "react";
import { Trash2 } from "lucide-react";
import { ErrorNote, Spinner } from "../ui";
import { api } from "../../lib/api";
import { fmtBytes, fmtRuntime, relPath } from "../../lib/format";
import { codecLabel, qualityLabel, qualityTone } from "../../lib/quality";
import type { DeleteResult, FileRole, ItemKind, ManageFile } from "../../lib/types";
import { errText } from "../../lib/errors";

const MOVIE_GROUPS: { role: FileRole; label: string }[] = [
  { role: "copy", label: "Copies" },
  { role: "part", label: "Parts" },
  { role: "extra", label: "Extras" },
];

/**
 * An item's files, grouped by episode, or for a movie into copies, parts and
 * extras. Where a movie or episode has several copies, the best is listed
 * first and the others can be deleted. A movie file can be marked as a part
 * (played in order as one movie) or an extra (bonus material).
 */
export default function FileList({
  kind,
  files,
  error,
  libraryPath,
  onDeleted,
  onChanged,
}: {
  kind: ItemKind;
  files?: ManageFile[];
  /** Why the files couldn't be loaded, when they couldn't. */
  error?: string | null;
  libraryPath: string;
  onDeleted: (r: DeleteResult) => void;
  onChanged: () => void;
}) {
  const [confirm, setConfirm] = useState<number | null>(null);
  const [busy, setBusy] = useState<number | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const isMovie = kind !== "series";

  const groups = new Map<string, ManageFile[]>();
  if (isMovie) for (const g of MOVIE_GROUPS) groups.set(g.role, []);
  for (const f of files ?? []) {
    const k = isMovie ? f.role : `${f.season ?? 0}x${f.episode ?? 0}`;
    groups.set(k, [...(groups.get(k) ?? []), f]);
  }
  for (const [k, g] of groups) if (!g.length) groups.delete(k);
  const copies = groups.get("copy")?.length ?? 0;
  const dupGroups = isMovie
    ? Number(copies > 1 || (copies > 0 && groups.has("part")))
    : [...groups.values()].filter((g) => g.length > 1).length;
  const maxPart = Math.max(0, ...(files ?? []).map((f) => f.partNo));

  const remove = async (f: ManageFile) => {
    setBusy(f.id);
    setErr(null);
    try {
      onDeleted(await api<DeleteResult>(`/api/files/${f.id}`, { method: "DELETE" }));
    } catch (e) {
      setErr(errText(e));
    } finally {
      setBusy(null);
      setConfirm(null);
    }
  };

  const setRole = async (f: ManageFile, role: FileRole, partNo = 0, extraTitle = "") => {
    setErr(null);
    try {
      await api(`/api/files/${f.id}/role`, { method: "PUT", json: { role, partNo, extraTitle } });
      onChanged();
    } catch (e) {
      setErr(errText(e));
    }
  };

  return (
    <section className="panel-section">
      <div className="mb-2 flex items-center gap-2">
        <div className="card-title">Files</div>
        {dupGroups > 0 && <span className="badge tint-warning">{isMovie ? "Duplicate copies" : `${dupGroups} with extra copies`}</span>}
      </div>
      {isMovie && files && files.length > 1 && (
        <p className="mb-2 text-xs text-content-muted">
          Not a duplicate? Mark a file as a part (the parts play one after another as one movie) or as an extra (listed under Extras).
        </p>
      )}
      {!files ? (
        error ? (
          <ErrorNote>Couldn't load the files: {error}</ErrorNote>
        ) : (
          <Spinner />
        )
      ) : (
        <div className="flex flex-col gap-2">
          {[...groups.entries()].map(([k, g]) => (
            <div key={k}>
              {!isMovie ? (
                <div className="mb-1 text-xs font-medium text-content-secondary">
                  S{g[0].season} · E{g[0].episode}
                  {g[0].episodeTitle && <span className="font-normal text-content-muted"> · {g[0].episodeTitle}</span>}
                </div>
              ) : (
                groups.size > 1 && <div className="mb-1 text-xs font-medium text-content-secondary">{MOVIE_GROUPS.find((m) => m.role === k)?.label}</div>
              )}
              {g.map((f, i) => {
                const dup = k === "copy" ? dupGroups > 0 && i > 0 : !isMovie && g.length > 1 && i > 0;
                return (
                  <div key={f.id} className={`flex items-start gap-2 rounded-md border px-2.5 py-1.5 ${dup ? "border-warning/40" : "border-border-light"} ${i > 0 ? "mt-1" : ""}`}>
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-1.5 text-xs">
                        <span className={`badge tint-${qualityTone(f.height)}`}>{qualityLabel(f.height)}</span>
                        <span className="text-content-secondary">
                          {[f.videoCodec && codecLabel(f.videoCodec), f.audioCodec?.toUpperCase(), f.container.toUpperCase(), fmtRuntime(f.durationSec), fmtBytes(f.size)]
                            .filter(Boolean)
                            .join(" · ")}
                        </span>
                        {(k === "copy" ? copies > 1 : !isMovie && g.length > 1) && i === 0 && <span className="badge tint-good">Best</span>}
                        {f.problem && <span className="badge tint-critical">{f.problem === "unreadable" ? "Damaged" : "No video"}</span>}
                      </div>
                      <div className="mono mt-0.5 truncate text-content-muted" title={f.path}>
                        {relPath(f.path, libraryPath)}
                      </div>
                      {isMovie && <RolePicker f={f} maxPart={maxPart} onSet={(...a) => void setRole(f, ...a)} />}
                    </div>
                    {confirm === f.id ? (
                      <div className="flex shrink-0 gap-1">
                        <button className="btn-danger !px-2 !py-1 !text-xs" disabled={busy !== null} onClick={() => void remove(f)}>
                          {busy === f.id ? <Spinner size={12} /> : "Delete file"}
                        </button>
                        <button className="btn-quiet !px-1.5 !text-xs" onClick={() => setConfirm(null)}>
                          Keep
                        </button>
                      </div>
                    ) : (
                      <button className="btn-quiet shrink-0 hover:!text-critical" onClick={() => setConfirm(f.id)} title="Delete this file from the NAS" aria-label="Delete file">
                        <Trash2 size={14} />
                      </button>
                    )}
                  </div>
                );
              })}
            </div>
          ))}
        </div>
      )}
      {err && (
        <div className="mt-2">
          <ErrorNote>{err}</ErrorNote>
        </div>
      )}
    </section>
  );
}

/** Copy / Part N / Extra for one movie file, with the extra's title. */
function RolePicker({ f, maxPart, onSet }: { f: ManageFile; maxPart: number; onSet: (role: FileRole, partNo?: number, extraTitle?: string) => void }) {
  const [title, setTitle] = useState(f.extraTitle);
  useEffect(() => setTitle(f.extraTitle), [f.extraTitle]);
  const value = f.role === "part" ? `part:${f.partNo}` : f.role;
  const parts = Array.from({ length: Math.max(4, maxPart + 1) }, (_, i) => i + 1);
  const saveTitle = () => title.trim() !== f.extraTitle && onSet("extra", 0, title);
  return (
    <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
      <select
        className="field !py-0.5 !text-xs"
        value={value}
        aria-label="What this file is"
        onChange={(e) => {
          const v = e.target.value;
          if (v.startsWith("part:")) onSet("part", Number(v.slice(5)));
          else if (v === "extra") onSet("extra", 0, f.extraTitle || "Bonus");
          else onSet("copy");
        }}
      >
        <option value="copy">Copy of the movie</option>
        {parts.map((n) => (
          <option key={n} value={`part:${n}`}>
            Part {n}
          </option>
        ))}
        <option value="extra">Extra (bonus)</option>
      </select>
      {f.role === "extra" && (
        <input
          className="field min-w-0 flex-1 !py-0.5 !text-xs"
          value={title}
          placeholder="Behind the Scenes"
          aria-label="Extra title"
          onChange={(e) => setTitle(e.target.value)}
          onBlur={saveTitle}
          onKeyDown={(e) => e.key === "Enter" && (e.target as HTMLInputElement).blur()}
        />
      )}
    </div>
  );
}
