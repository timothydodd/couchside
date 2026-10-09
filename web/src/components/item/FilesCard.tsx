import { Trash2 } from "lucide-react";
import Link from "../Link";
import { Meter } from "../ui";
import { api } from "../../lib/api";
import { confirmDialog } from "../../lib/ask";
import { fmtBytes, fmtResolution, fmtRuntime } from "../../lib/format";
import { attempt } from "../../lib/notices";
import { canDirectPlay } from "../../lib/playback";
import { PROBLEM_TEXT, type MediaFile } from "../../lib/types";
import { useIsAdmin } from "../../stores/auth";
import RangeChip from "../RangeChip";

/** How this browser will play a file. */
export function PlaybackChip({ f }: { f: MediaFile }) {
  if (f.problem)
    return (
      <span className="badge tint-warning" title={PROBLEM_TEXT[f.problem].detail}>
        {PROBLEM_TEXT[f.problem].label}
      </span>
    );
  if (canDirectPlay(f)) return <span className="badge tint-good">Direct play</span>;
  if (f.optimized) return <span className="badge tint-good">Optimized</span>;
  return (
    <span className="badge tint-info" title="The browser can't play this format as is; it will be converted while you watch">
      Transcode
    </span>
  );
}

/** A title's or episode's copies: quality, codecs, size, how they'll play and how far they've been watched. */
export default function FilesCard({ files, onChange }: { files: MediaFile[]; onChange: () => void }) {
  const admin = useIsAdmin();
  const dropOptimized = attempt("Couldn't delete the optimized copy", async (id: number) => {
    if (!(await confirmDialog({ title: "Delete the optimized copy?", body: "Playback falls back to the original file or a server stream.", action: "Delete", danger: true }))) return;
    await api(`/api/files/${id}/optimized`, { method: "DELETE" });
    onChange();
  });
  return (
    <section className="mt-10 gutter">
      <h2 className="row-title mb-3">{files.length === 1 ? "File" : `Files (${files.length})`}</h2>
      <div className="card overflow-x-auto">
        <table className="table">
          <thead>
            <tr>
              <th>Name</th>
              <th>Quality</th>
              <th className="hidden sm:table-cell">Video</th>
              <th className="hidden sm:table-cell">Audio</th>
              <th className="hidden sm:table-cell">Length</th>
              <th className="text-right">Size</th>
              <th>Playback</th>
              <th>Progress</th>
            </tr>
          </thead>
          <tbody>
            {files.map((f) => (
              <tr key={f.id}>
                <td className="mono max-w-xs truncate" title={f.path}>
                  {f.role === "part" && <span className="chip mr-1.5">Part {f.partNo}</span>}
                  <Link to={`/play/${f.id}`} className="hover:text-accent">
                    {f.path}
                  </Link>
                </td>
                <td>
                  <span className="inline-flex flex-wrap items-center gap-1">
                    {fmtResolution(f.width, f.height) && <span className="chip">{fmtResolution(f.width, f.height)}</span>}
                    <RangeChip range={f.dynamicRange} dvProfile={f.dvProfile} />
                  </span>
                </td>
                <td className="mono hidden text-content-secondary sm:table-cell">{f.videoCodec || "?"}</td>
                <td className="mono hidden text-content-secondary sm:table-cell">
                  {f.audioCodec || "?"}
                  {f.audioTracks > 1 && <span className="text-content-muted"> +{f.audioTracks - 1}</span>}
                </td>
                <td className="hidden tabular-nums text-content-secondary sm:table-cell">{fmtRuntime(f.durationSec)}</td>
                <td className="text-right tabular-nums text-content-secondary">{fmtBytes(f.size)}</td>
                <td>
                  <span className="inline-flex items-center gap-1">
                    <PlaybackChip f={f} />
                    {f.optimized && admin && (
                      <button className="btn-quiet !p-1 hover:!text-critical" title="Delete the optimized copy" onClick={() => void dropOptimized(f.id)}>
                        <Trash2 size={12} />
                      </button>
                    )}
                  </span>
                </td>
                <td className="w-32">
                  {f.watched ? (
                    <span className="text-xs text-good">Watched</span>
                  ) : f.durationSec && f.positionSec > 0 ? (
                    <Meter value={(f.positionSec / f.durationSec) * 100} />
                  ) : (
                    <span className="text-xs text-content-muted">—</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
