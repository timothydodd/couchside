import { useEffect, useRef, useState, type InputHTMLAttributes, type ReactNode, type Ref } from "react";
import { Loader2, Search } from "lucide-react";
import Link from "./Link";

// Shared primitives, ported from Portside Lite's components/ui.tsx so both
// apps look and behave the same.

export type Tone = "good" | "warning" | "critical" | "info" | "muted";

export function StatusPill({ label, tone, pulse }: { label: string; tone: Tone; pulse?: boolean }) {
  const dot = {
    good: "bg-good",
    critical: "bg-critical",
    warning: "bg-warning",
    info: "bg-info",
    muted: "bg-content-muted",
  }[tone];
  return (
    <span className="inline-flex items-center gap-1.5 whitespace-nowrap text-xs text-content-secondary">
      <span className={`h-2 w-2 rounded-full ${dot} ${pulse ? "animate-pulse" : ""}`} />
      {label}
    </span>
  );
}

/** Progress meter: accent fill on a lighter track of the same hue. */
export function Meter({ value, className = "" }: { value: number; className?: string }) {
  const v = Math.max(0, Math.min(100, value));
  return (
    <div
      className={`h-1.5 w-full overflow-hidden rounded-full ${className}`}
      style={{ backgroundColor: "color-mix(in srgb, var(--accent) 18%, transparent)" }}
      role="meter"
      aria-valuenow={Math.round(v)}
      aria-valuemin={0}
      aria-valuemax={100}
    >
      <div className="h-full rounded-full bg-accent transition-[width] duration-500" style={{ width: `${v}%` }} />
    </div>
  );
}

export function StatTile({
  label,
  value,
  sub,
  tone,
  meter,
  onClick,
}: {
  label: string;
  value: ReactNode;
  sub?: ReactNode;
  tone?: "critical" | "warning" | "good";
  /** 0–100: draws a fill bar under the value. */
  meter?: number;
  onClick?: () => void;
}) {
  const ring = tone === "critical" ? "border-critical/50" : tone === "warning" ? "border-warning/50" : "border-border-light";
  const Tag = onClick ? "button" : "div";
  return (
    <Tag
      onClick={onClick}
      className={`card flex flex-col items-start gap-1 border px-4 py-3 text-left ${ring} ${onClick ? "transition-colors hover:border-accent" : ""}`}
    >
      <span className="text-xs text-content-muted">{label}</span>
      <span className="text-2xl font-semibold tabular-nums text-content">{value}</span>
      {meter !== undefined && <Meter value={meter} className="my-0.5" />}
      {sub && <span className="text-xs text-content-secondary">{sub}</span>}
    </Tag>
  );
}

export function PageHeader({ title, subtitle, children }: { title: string; subtitle?: ReactNode; children?: ReactNode }) {
  return (
    <div className="flex flex-wrap items-end justify-between gap-3 border-b border-border-light gutter pb-3 pt-5">
      <div>
        <h1 className="text-lg font-semibold text-content">{title}</h1>
        {subtitle && <p className="mt-0.5 text-xs text-content-muted">{subtitle}</p>}
      </div>
      {children && <div className="flex w-full flex-wrap items-center gap-2 sm:w-auto">{children}</div>}
    </div>
  );
}

export function EmptyState({ icon, title, children }: { icon?: ReactNode; title: string; children?: ReactNode }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 gutter py-16 text-center">
      {icon && <div className="text-content-muted">{icon}</div>}
      <div className="text-sm font-medium text-content">{title}</div>
      {children && <div className="max-w-md text-xs text-content-muted">{children}</div>}
    </div>
  );
}

/** A row of mutually exclusive choices (theme, quality…). */
export function Segmented<T extends string | number>({
  value,
  options,
  onChange,
  label,
}: {
  value: T;
  options: { id: T; label: string }[];
  onChange: (v: T) => void;
  label: string;
}) {
  return (
    <div className="inline-flex rounded-md border border-border p-0.5" role="radiogroup" aria-label={label}>
      {options.map((o) => (
        <button
          key={o.id}
          type="button"
          role="radio"
          aria-checked={value === o.id}
          onClick={() => onChange(o.id)}
          className={`rounded px-3 py-1 text-sm transition-colors ${value === o.id ? "bg-accent text-on-accent" : "text-content-secondary hover:text-content"}`}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

export function Spinner({ size = 16 }: { size?: number }) {
  return <Loader2 size={size} className="animate-spin text-content-muted" />;
}

export function ErrorNote({ children }: { children: ReactNode }) {
  return <div className="tint-critical rounded-md px-3 py-2 text-xs">{children}</div>;
}

export function SearchInput({
  value,
  onChange,
  placeholder = "Filter…",
  className = "",
  large = false,
  ...input
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  className?: string;
  /** The page's main search: taller, bigger text. */
  large?: boolean;
} & Omit<InputHTMLAttributes<HTMLInputElement>, "value" | "onChange" | "placeholder" | "className"> & {
    ref?: Ref<HTMLInputElement>;
  }) {
  return (
    <div className={`relative ${className}`}>
      <Search
        size={large ? 18 : 14}
        className={`pointer-events-none absolute top-1/2 -translate-y-1/2 text-content-muted ${large ? "left-3.5" : "left-2.5"}`}
      />
      <input
        {...input}
        className={`field w-full ${large ? "h-11 pl-10 text-base" : "pl-8"}`}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
      />
    </div>
  );
}

export interface MenuItem {
  id: string;
  label: string;
  detail?: string;
  icon?: ReactNode;
  /** Destructive: shown in the critical colour. */
  danger?: boolean;
  onSelect: () => void;
}

/** A small button that opens a list of less-used actions. Closes on pick, outside click or Escape. */
/**
 * An icon button that opens a menu. align="end" lines the menu up with the
 * button's right edge (for buttons near the right of the screen); className
 * styles the button (e.g. the right half of a split button).
 */
export function MenuButton({
  label,
  icon,
  items,
  align = "start",
  className = "btn-ghost !px-2 !py-2",
}: {
  label: string;
  icon: ReactNode;
  items: MenuItem[];
  align?: "start" | "end";
  className?: string;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => !ref.current?.contains(e.target as Node) && setOpen(false);
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);
  return (
    <div ref={ref} className="relative inline-flex">
      <button className={className} aria-label={label} title={label} aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
        {icon}
      </button>
      {open && (
        <div role="menu" className={`menu ${align === "end" ? "menu-end" : ""}`}>
          {items.map((it) => (
            <button
              key={it.id}
              role="menuitem"
              className={`menu-item ${it.danger ? "menu-item-danger" : ""}`}
              onClick={() => {
                setOpen(false);
                it.onSelect();
              }}
            >
              <span className="flex items-center gap-2">
                {it.icon}
                {it.label}
              </span>
              {it.detail && <span className={`menu-detail ${it.icon ? "pl-6" : ""}`}>{it.detail}</span>}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

/** A job-queued message; the word "Activity" in it links to the Activity page. */
export function JobNote({ msg, className = "text-xs text-content-muted" }: { msg: string; className?: string }) {
  const [before, after] = msg.split("Activity");
  return (
    <span className={className}>
      {after === undefined ? (
        msg
      ) : (
        <>
          {before}
          <Link to="/activity" className="text-accent hover:underline">
            Activity
          </Link>
          {after}
        </>
      )}
    </span>
  );
}
