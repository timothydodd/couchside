import { useRef, useState, type ReactNode } from "react";
import { SlidersHorizontal, X } from "lucide-react";
import { radioKeys, useDialog } from "../lib/dialog";
import { usePhone } from "../lib/media";

/**
 * A "Filters" button that opens a panel of choices: a dropdown under the
 * button from md up, a bottom sheet on phones. active is how many filters
 * differ from their defaults (shown on the button); onReset clears them.
 */
export default function FilterMenu({ active, onReset, children }: { active: number; onReset?: () => void; children: ReactNode }) {
  const [open, setOpen] = useState(false);

  return (
    <div className="relative shrink-0">
      <button
        type="button"
        className={`btn-ghost h-11 ${active ? "!border-accent text-accent" : ""}`}
        onClick={() => setOpen((o) => !o)}
        aria-expanded={open}
        aria-haspopup="dialog"
        aria-label={active ? `Filters (${active} on)` : "Filters"}
      >
        <SlidersHorizontal size={16} />
        <span className="hidden sm:inline">Filters</span>
        {active > 0 && <span className="rounded-full bg-accent px-1.5 text-[11px] font-semibold text-on-accent">{active}</span>}
      </button>
      {open && (
        <Panel active={active} onReset={onReset} onClose={() => setOpen(false)}>
          {children}
        </Panel>
      )}
    </div>
  );
}

/** The open panel. Its own component so the dialog behaviour lasts exactly as long as it's open. */
function Panel({ active, onReset, onClose, children }: { active: number; onReset?: () => void; onClose: () => void; children: ReactNode }) {
  const ref = useRef<HTMLDivElement>(null);
  useDialog(ref, onClose); // focus in, Tab kept inside, Escape, focus back to the button
  const phone = usePhone();
  return (
    <>
      <div className="anim-fade fixed inset-0 z-40 bg-backdrop/55 md:bg-transparent" onClick={onClose} />
      <div ref={ref} role="dialog" aria-modal={phone || undefined} aria-label="Filters" className={`filter-panel ${phone ? "anim-slide-up" : "anim-pop"}`}>
        <div className="flex items-center justify-between gap-2 px-4 pt-4">
          <div className="card-title">Filters</div>
          <div className="flex items-center gap-1">
            {onReset && active > 0 && (
              <button type="button" className="btn-quiet !text-xs" onClick={onReset}>
                Reset
              </button>
            )}
            <button type="button" className="touch-target text-content-muted md:hidden" onClick={onClose} aria-label="Close">
              <X size={20} />
            </button>
          </div>
        </div>
        <div className="flex flex-col gap-4 p-4">{children}</div>
      </div>
    </>
  );
}

/** One group of mutually exclusive choices, shown as chips. */
export function Choices<T extends string>({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: T;
  options: { id: T; label: string }[];
  onChange: (v: T) => void;
}) {
  return (
    <div role="radiogroup" aria-label={label} onKeyDown={radioKeys}>
      <div className="field-label">{label}</div>
      <div className="flex flex-wrap gap-1.5">
        {options.map((o) => (
          <button key={o.id} type="button" role="radio" aria-checked={o.id === value} tabIndex={o.id === value ? 0 : -1} className="choice" onClick={() => onChange(o.id)}>
            {o.label}
          </button>
        ))}
      </div>
    </div>
  );
}

/** An active filter shown under the search box; tap to clear it. */
export function ActiveChip({ label, onClear }: { label: string; onClear: () => void }) {
  return (
    <button type="button" onClick={onClear} className="choice choice-on inline-flex items-center gap-1 !min-h-7 !text-xs" aria-label={`Clear ${label}`}>
      {label}
      <X size={12} />
    </button>
  );
}
