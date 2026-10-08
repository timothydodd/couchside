import { ChevronRight, CornerLeftUp, Folder, HardDrive } from "lucide-react";
import { useApi } from "../lib/api";
import type { Browse } from "../lib/types";

/**
 * Browse the server's folders. Inside the media folder when one is set
 * (anywhere with `all`, for choosing the media folder itself); otherwise from
 * the drives on Windows, or /. Picking a folder sets it and opens it.
 */
export default function FolderPicker({ path, onPick, all = false }: { path: string; onPick: (p: string) => void; all?: boolean }) {
  const q = `/api/fs?path=${encodeURIComponent(path.trim())}${all ? "&all=1" : ""}`;
  const { data, error } = useApi<Browse>(q, { keep: true });
  if (error)
    return <div className="mt-2 rounded-md border border-border-light bg-input px-3 py-2 text-xs text-content-muted">{error}. Type the path, or clear it to start over.</div>;
  if (!data) return null;
  const top = data.path === "";
  return (
    <div className="mt-2 overflow-hidden rounded-md border border-border-light bg-input">
      <div className="mono truncate border-b border-border-light px-3 py-1.5 text-xs text-content-muted">{top ? "This server" : data.path}</div>
      <div className="max-h-56 overflow-auto">
        {data.parent !== null && (
          <button className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm text-content-muted hover:bg-muted hover:text-content" onClick={() => onPick(data.parent!)}>
            <CornerLeftUp size={14} /> {data.parent === "" ? "All drives" : ".."}
          </button>
        )}
        {data.dirs.map((d) => (
          <button
            key={d.path}
            className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm text-content-secondary hover:bg-muted hover:text-content"
            onClick={() => onPick(d.path)}
          >
            {top ? <HardDrive size={14} className="text-accent" /> : <Folder size={14} className="text-accent" />}
            <span className="flex-1 truncate">{d.name}</span>
            <ChevronRight size={14} className="text-content-muted" />
          </button>
        ))}
        {data.dirs.length === 0 && <div className="px-3 py-2 text-xs text-content-muted">No subfolders</div>}
      </div>
    </div>
  );
}
