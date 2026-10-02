import { useRef, useState } from "react";
import { RotateCcw, Upload } from "lucide-react";
import { ErrorNote, Spinner } from "../ui";
import { api, backdropUrl, posterUrl } from "../../lib/api";
import type { ManageRow } from "../../lib/types";

type Kind = "poster" | "backdrop";

/** Upload your own poster or backdrop, or go back to the automatic one. */
export default function ArtworkEditor({ row, onChanged }: { row: ManageRow; onChanged: () => void }) {
  const [busy, setBusy] = useState<Kind | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const upload = async (kind: Kind, file: File) => {
    setBusy(kind);
    setErr(null);
    try {
      // Through api(), so an expired access token is renewed and the upload retried.
      await api(`/api/items/${row.id}/artwork/${kind}`, { method: "PUT", headers: { "Content-Type": file.type || "application/octet-stream" }, body: file });
      onChanged();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(null);
    }
  };
  const reset = async (kind: Kind) => {
    setBusy(kind);
    setErr(null);
    try {
      await api(`/api/items/${row.id}/artwork/${kind}`, { method: "DELETE" });
      // The artwork job runs in the background; refresh once it's likely done.
      onChanged();
      setTimeout(onChanged, 4000);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(null);
    }
  };

  return (
    <section className="panel-section">
      <div className="card-title mb-3">Artwork</div>
      <div className="grid grid-cols-[88px_1fr] gap-4">
        <Slot
          label="Poster"
          kind="poster"
          custom={row.customPoster}
          busy={busy === "poster"}
          preview={row.hasPoster ? posterUrl(row) : null}
          aspect="aspect-[2/3]"
          onUpload={upload}
          onReset={reset}
        />
        <Slot
          label="Backdrop"
          kind="backdrop"
          custom={row.customBackdrop}
          busy={busy === "backdrop"}
          preview={backdropUrl(row)}
          aspect="aspect-video"
          onUpload={upload}
          onReset={reset}
        />
      </div>
      {err && (
        <div className="mt-3">
          <ErrorNote>{err}</ErrorNote>
        </div>
      )}
      <p className="mt-3 text-xs text-content-muted">JPEG, PNG or WebP up to 25 MB. Uploaded artwork stays when the match or library changes.</p>
    </section>
  );
}

function Slot(p: {
  label: string;
  kind: Kind;
  custom: boolean;
  busy: boolean;
  preview: string | null;
  aspect: string;
  onUpload: (kind: Kind, f: File) => void;
  onReset: (kind: Kind) => void;
}) {
  const input = useRef<HTMLInputElement>(null);
  const [broken, setBroken] = useState(false);
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <div className={`${p.aspect} relative overflow-hidden rounded-md border border-border-light bg-raised`}>
        {p.preview && !broken && <img src={p.preview} alt="" className="h-full w-full object-cover" onError={() => setBroken(true)} onLoad={() => setBroken(false)} />}
        {p.busy && (
          <div className="absolute inset-0 flex items-center justify-center bg-black/40">
            <Spinner size={18} />
          </div>
        )}
      </div>
      <div className="flex flex-wrap items-center gap-1">
        <span className="text-xs text-content-secondary">{p.label}</span>
        {p.custom && <span className="badge tint-info">Yours</span>}
      </div>
      <div className="flex flex-wrap gap-1">
        <button className="btn-chip" disabled={p.busy} onClick={() => input.current?.click()}>
          <Upload size={12} /> Upload
        </button>
        {p.custom && (
          <button className="btn-chip" disabled={p.busy} onClick={() => p.onReset(p.kind)} title="Use the automatic artwork again">
            <RotateCcw size={12} /> Automatic
          </button>
        )}
      </div>
      <input
        ref={input}
        type="file"
        accept="image/*"
        hidden
        onChange={(e) => {
          const f = e.target.files?.[0];
          e.target.value = "";
          if (f) p.onUpload(p.kind, f);
        }}
      />
    </div>
  );
}
