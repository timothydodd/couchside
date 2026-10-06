import { useEffect, useRef } from "react";
import { Play, X } from "lucide-react";
import type { QueueEntry } from "../../stores/queue";

/**
 * The play queue over the player: what's playing, what's next, and what
 * came before. Click to jump, × to drop something. The player pauses while
 * it's open and carries on when it closes.
 */
export default function QueueMenu({
  entries,
  current,
  onPlay,
  onRemove,
  onClose,
}: {
  entries: QueueEntry[];
  current: number;
  onPlay: (fileId: number) => void;
  onRemove: (fileId: number) => void;
  onClose: () => void;
}) {
  const list = useRef<HTMLDivElement>(null);
  // Start at the entry playing, with a little of what came before in view.
  useEffect(() => {
    list.current?.querySelector<HTMLElement>("[data-current]")?.scrollIntoView({ block: "center" });
  }, []);
  const at = entries.findIndex((e) => e.fileId === current);
  return (
    <div
      className="card absolute bottom-16 right-3 z-30 flex max-h-[70vh] w-96 max-w-[calc(100vw-1.5rem)] flex-col p-1.5 text-sm shadow-[var(--shadow-md)]"
      onClick={(e) => e.stopPropagation()}
      onDoubleClick={(e) => e.stopPropagation()}
      role="dialog"
      aria-label="Play queue"
    >
      <div className="flex items-center gap-2 px-3 py-2">
        <span className="card-title flex-1 !text-content-secondary">Queue</span>
        <span className="text-xs text-content-muted">
          {at >= 0 ? `${at + 1} of ${entries.length}` : `${entries.length}`}
        </span>
        <button className="btn-quiet !p-1" onClick={onClose} aria-label="Close">
          <X size={14} />
        </button>
      </div>
      <div className="my-1 border-t border-border-light" />
      <div className="min-h-0 flex-1 overflow-y-auto" ref={list}>
        {entries.map((e, i) => {
          const playing = e.fileId === current;
          return (
            <div key={e.fileId} className={`group flex items-center gap-1 rounded ${playing ? "bg-accent/15" : "hover:bg-muted"}`} data-current={playing || undefined}>
              <button className="flex min-w-0 flex-1 items-center gap-3 px-3 py-2 text-left" onClick={() => !playing && onPlay(e.fileId)} aria-current={playing || undefined}>
                <span className={`w-5 shrink-0 text-right text-xs tabular-nums ${playing ? "text-accent" : "text-content-muted"}`}>{playing ? <Play size={12} className="ml-auto fill-current" /> : i + 1}</span>
                <span className="min-w-0 flex-1">
                  <span className={`block truncate ${playing ? "text-accent" : "text-content"}`}>{e.title}</span>
                  {e.subtitle && <span className="block truncate text-xs text-content-muted">{e.subtitle}</span>}
                </span>
              </button>
              {!playing && (
                <button className="btn-quiet mr-1 !p-1 opacity-0 group-hover:opacity-100 focus:opacity-100 pointer-coarse:opacity-100" onClick={() => onRemove(e.fileId)} aria-label={`Remove ${e.title} from the queue`}>
                  <X size={14} />
                </button>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
