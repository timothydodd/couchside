import { X } from "lucide-react";
import { useNotices } from "../lib/notices";

/** Failed actions, over everything (the player included). */
export default function Notices() {
  const { list, dismiss } = useNotices();
  if (!list.length) return null;
  return (
    <div className="notice-stack" role="status" aria-live="polite">
      {list.map((n) => (
        <div key={n.id} className="notice">
          <span className="min-w-0 flex-1">{n.text}</span>
          <button className="btn-quiet !p-0.5" onClick={() => dismiss(n.id)} aria-label="Dismiss">
            <X size={14} />
          </button>
        </div>
      ))}
    </div>
  );
}
