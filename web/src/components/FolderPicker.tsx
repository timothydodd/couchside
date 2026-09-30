import { ChevronRight, CornerLeftUp, Folder } from "lucide-react";
import { useApi } from "../lib/api";
import type { Browse } from "../lib/types";

/** Browse subfolders under the server's media root. */
export default function FolderPicker({ path, onPick }: { path: string; onPick: (p: string) => void }) {
  const { data, error } = useApi<Browse>(`/api/fs?path=${encodeURIComponent(path)}`);
  if (error) return null; // typed path isn't browsable; the text field still works
  if (!data) return null;
  return (
    <div className="mt-2 max-h-56 overflow-auto rounded-md border border-border-light bg-input">
      {data.parent && (
        <button className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm text-content-muted hover:bg-muted hover:text-content" onClick={() => onPick(data.parent!)}>
          <CornerLeftUp size={14} /> ..
        </button>
      )}
      {data.dirs.map((d) => (
        <button
          key={d}
          className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-sm text-content-secondary hover:bg-muted hover:text-content"
          onClick={() => onPick(`${data.path.replace(/\/$/, "")}/${d}`)}
        >
          <Folder size={14} className="text-accent" />
          <span className="flex-1 truncate">{d}</span>
          <ChevronRight size={14} className="text-content-muted" />
        </button>
      ))}
      {data.dirs.length === 0 && <div className="px-3 py-2 text-xs text-content-muted">No subfolders</div>}
    </div>
  );
}
