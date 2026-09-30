import { useState } from "react";
import { Trash2 } from "lucide-react";
import { ErrorNote, Spinner } from "../ui";
import { api } from "../../lib/api";
import { fmtBytes, fmtRuntime, relPath } from "../../lib/format";
import { codecLabel, qualityLabel, qualityTone } from "../../lib/quality";
import type { DeleteResult, ItemKind, ManageFile } from "../../lib/types";

/**
 * An item's files, grouped by episode. Where a movie or episode has several
 * copies, the best is listed first and the others can be deleted.
 */
export default function FileList({ kind, files, libraryPath, onDeleted }: { kind: ItemKind; files?: ManageFile[]; libraryPath: string; onDeleted: (r: DeleteResult) => void }) {
  const [confirm, setConfirm] = useState<number | null>(null);
  const [busy, setBusy] = useState<number | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const groups = new Map<string, ManageFile[]>();
  for (const f of files ?? []) {
    const k = kind === "series" ? `${f.season ?? 0}x${f.episode ?? 0}` : "movie";
    groups.set(k, [...(groups.get(k) ?? []), f]);
  }
  const dupGroups = [...groups.values()].filter((g) => g.length > 1).length;

  const remove = async (f: ManageFile) => {
    setBusy(f.id);
    setErr(null);
    try {
      onDeleted(await api<DeleteResult>(`/api/files/${f.id}`, { method: "DELETE" }));
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(null);
      setConfirm(null);
    }
  };

  return (
    <section className="panel-section">
      <div className="mb-2 flex items-center gap-2">
        <div className="card-title">Files</div>
        {dupGroups > 0 && <span className="badge tint-warning">{dupGroups === 1 && kind !== "series" ? "Duplicate copies" : `${dupGroups} with extra copies`}</span>}
      </div>
      {!files ? (
        <Spinner />
      ) : (
        <div className="flex flex-col gap-2">
          {[...groups.entries()].map(([k, g]) => (
            <div key={k}>
              {kind === "series" && (
                <div className="mb-1 text-xs font-medium text-content-secondary">
                  S{g[0].season} · E{g[0].episode}
                  {g[0].episodeTitle && <span className="font-normal text-content-muted"> · {g[0].episodeTitle}</span>}
                </div>
              )}
              {g.map((f, i) => (
                <div key={f.id} className={`flex items-start gap-2 rounded-md border px-2.5 py-1.5 ${g.length > 1 && i > 0 ? "border-warning/40" : "border-border-light"}`}>
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-1.5 text-xs">
                      <span className={`badge tint-${qualityTone(f.height)}`}>{qualityLabel(f.height)}</span>
                      <span className="text-content-secondary">
                        {[f.videoCodec && codecLabel(f.videoCodec), f.audioCodec?.toUpperCase(), f.container.toUpperCase(), fmtRuntime(f.durationSec), fmtBytes(f.size)]
                          .filter(Boolean)
                          .join(" · ")}
                      </span>
                      {g.length > 1 && i === 0 && <span className="badge tint-good">Best</span>}
                      {f.problem && <span className="badge tint-critical">{f.problem === "unreadable" ? "Damaged" : "No video"}</span>}
                    </div>
                    <div className="mono mt-0.5 truncate text-content-muted" title={f.path}>
                      {relPath(f.path, libraryPath)}
                    </div>
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
              ))}
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
