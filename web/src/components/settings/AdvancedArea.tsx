import { useState, type ReactNode } from "react";
import { ChevronRight } from "lucide-react";

/**
 * A settings page's advanced settings, folded away under its everyday cards
 * so the page leads with what most people change. `what` says what's inside.
 */
export default function AdvancedArea({ what, children }: { what: string; children: ReactNode }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="border-t border-border-light pt-2">
      <button type="button" className="advanced-toggle" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
        <ChevronRight size={14} className={`shrink-0 transition-transform ${open ? "rotate-90" : ""}`} />
        Advanced
        <span className="truncate font-normal normal-case tracking-normal">{what}</span>
      </button>
      {open && <div className="mt-2 flex flex-col gap-4">{children}</div>}
    </div>
  );
}
